"use client";

import { useMemo, useState, useEffect } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import useSWR from "swr";
import { IconArrowLeft } from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import PodcastCover from "@/components/podcasts/PodcastCover";
import RichText from "@/components/RichText";
import RefreshCollectionModal from "@/components/collections/RefreshCollectionModal";
import { positiveID } from "@/lib/navigation";
import { requestTypedConfirmation } from "@/lib/confirmation";
import {
  ADOPTED_FILTERS,
  adoptCollectionItem,
  collectionErrorMessage,
  collectionErrorCode,
  deleteCollection,
  fetchCollectionDetail,
  filterItemsByAdoptedState,
  formatCollectionDate,
  formatCollectionDuration,
} from "@/lib/collections";
import type {
  CollectionAdoptedFilter,
  CollectionItemDetail,
} from "@/types/collection";

const QUEUE_LABELS: Record<string, string> = {
  inbox: "已在 Inbox",
  focus: "已在 Focus",
  someday: "已在 Someday",
  done: "已完成",
};

function adoptedState(item: CollectionItemDetail): string {
  if (!item.adopted_episode_id) return "未收录";
  if (item.adopted_episode_queue && QUEUE_LABELS[item.adopted_episode_queue]) {
    return QUEUE_LABELS[item.adopted_episode_queue];
  }
  if (item.adopted_episode_dismissed_at) return "不感兴趣";
  return "已收录";
}

interface CollectionDetailContentProps {
  collectionID: number;
}

/**
 * 清单详情：保留原始顺序与逐集推荐语，区分推荐语、Show Notes 与
 * 真实个人收录状态。地址直达与刷新只读取，不写库、不触发加工。
 */
export default function CollectionDetailContent({
  collectionID,
}: CollectionDetailContentProps) {
  const [filter, setFilter] = useState<CollectionAdoptedFilter>("all");
  const params = useSearchParams();
  const query = params.toString();
  const [selectedItemID, setSelectedItemID] = useState<number | null>(
    positiveID(params.get("item")) ?? null,
  );
  const [adoptingItemIDs, setAdoptingItemIDs] = useState<Set<number>>(
    () => new Set(),
  );
  const [readbackNotice, setReadbackNotice] = useState("");
  useEffect(() => {
    const parsed = new URLSearchParams(query);
    const requested = parsed.get("filter");
    setFilter(
      requested === "adopted" || requested === "unadopted" ? requested : "all",
    );
    setSelectedItemID(positiveID(parsed.get("item")) ?? null);
  }, [query, collectionID]);
  const [adoptError, setAdoptError] = useState<{
    itemID: number;
    message: string;
  } | null>(null);
  const [isRefreshOpen, setRefreshOpen] = useState(false);
  const [refreshNotice, setRefreshNotice] = useState("");
  const [isDeleting, setDeleting] = useState(false);
  const router = useRouter();

  const { data, error, isLoading, mutate } = useSWR(
    collectionID > 0 ? `/api/v1/collections/${collectionID}` : null,
    () => fetchCollectionDetail(collectionID),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );

  useEffect(() => {
    if (selectedItemID && data) {
      document
        .getElementById(`collection-item-${selectedItemID}`)
        ?.scrollIntoView?.({ block: "center" });
    }
  }, [selectedItemID, data, filter]);

  const confirmDelete = () => {
    const ack = requestTypedConfirmation({
      action: "删除清单",
      impact:
        "只删除清单及其未收录条目；已收录单集、队列、笔记与来源记录全部保留。",
      phrase: "删除清单",
    });
    if (!ack || isDeleting) return;
    setDeleting(true);
    void deleteCollection(collectionID)
      .then(() => {
        router.push("/collections");
      })
      .catch(() => {
        setDeleting(false);
        setRefreshNotice("删除失败，清单保持原样，可稍后重试。");
      });
  };

  const items = useMemo(() => data?.items ?? [], [data]);
  const visibleItems = useMemo(
    () => filterItemsByAdoptedState(items, filter),
    [items, filter],
  );

  const adopt = async (item: CollectionItemDetail) => {
    if (adoptingItemIDs.has(item.id)) return;
    setAdoptingItemIDs((current) => new Set([...current, item.id]));
    setAdoptError(null);
    try {
      // 收录后以服务端返回的真实状态刷新详情，不本地伪造队列状态。
      const result = await adoptCollectionItem(collectionID, item.id);
      await mutate((current) => {
        if (!current) return current;
        const updated = current.items.map((entry) =>
          entry.id === item.id
            ? {
                ...entry,
                adopted_episode_id: result.episode_id,
                adopted_episode_queue: result.queue_state,
                adopted_episode_dismissed_at: result.dismissed_at,
              }
            : entry,
        );
        return {
          ...current,
          items: updated,
          adopted_count: updated.filter(
            (entry) => entry.adopted_episode_id !== null,
          ).length,
        };
      }, false);
      setReadbackNotice("");
      void mutate().catch(() =>
        setReadbackNotice("已收录，最新清单暂时无法读取，可重试。"),
      );
    } catch (caught) {
      const code = collectionErrorCode(caught);
      setAdoptError({
        itemID: item.id,
        message:
          code === "EPISODE_DELETED"
            ? "这一集曾从个人库删除，收录不会自动恢复。"
            : collectionErrorMessage(caught, "收录失败，可稍后重试。"),
      });
    } finally {
      setAdoptingItemIDs((current) => {
        const next = new Set(current);
        next.delete(item.id);
        return next;
      });
    }
  };

  if (collectionID === 0) {
    return (
      <PageLayout
        rootClassName="editorial-page-shell podcast-library-shell"
        className="collection-page"
      >
        <div className="collection-empty" role="alert">
          <h3>清单地址无效</h3>
          <button
            type="button"
            className="collection-btn-secondary"
            onClick={() => void mutate()}
          >
            重新尝试
          </button>
          <Link href="/collections" className="collection-btn-secondary">
            返回播客清单
          </Link>
        </div>
      </PageLayout>
    );
  }

  if (error && !data) {
    return (
      <PageLayout
        rootClassName="editorial-page-shell podcast-library-shell"
        className="collection-page"
      >
        <div className="collection-empty" role="alert">
          <h3>清单暂时无法读取</h3>
          <p>{collectionErrorMessage(error, "连接未成功，可稍后重试。")}</p>
          <button
            type="button"
            className="collection-btn-secondary"
            onClick={() => void mutate()}
          >
            重新尝试
          </button>
          <Link href="/collections" className="collection-btn-secondary">
            返回播客清单
          </Link>
        </div>
      </PageLayout>
    );
  }

  if (isLoading || !data) {
    return (
      <PageLayout
        rootClassName="editorial-page-shell podcast-library-shell"
        className="collection-page"
      >
        <div className="collection-empty" aria-live="polite" aria-busy="true">
          <h3>正在读取清单…</h3>
        </div>
      </PageLayout>
    );
  }

  const failedFilterCount = items.length - visibleItems.length;

  return (
    <PageLayout
      rootClassName="editorial-page-shell podcast-library-shell"
      className="collection-page"
      toolbar={{
        title: data.title,
        description: `作者：${data.author || "未提供"} · ${
          data.total_known
            ? `共 ${data.item_count} 集`
            : `已读取 ${data.item_count} 集`
        } · 已收录 ${data.adopted_count} 集`,
        mobileDescription: `作者：${data.author || "未提供"} · 已收录 ${data.adopted_count} 集`,
        rightContent: (
          <div className="collection-toolbar-actions">
            <Link href="/collections" className="collection-btn-secondary">
              <IconArrowLeft aria-hidden="true" stroke={1.8} />
              <span>返回列表</span>
            </Link>
            <button
              type="button"
              className="collection-btn-secondary"
              onClick={() => setRefreshOpen(true)}
            >
              刷新清单
            </button>
            <button
              type="button"
              className="collection-btn-secondary"
              disabled={isDeleting}
              onClick={confirmDelete}
            >
              {isDeleting ? "正在删除…" : "删除清单"}
            </button>
            <a
              href={data.source_url}
              target="_blank"
              rel="noopener noreferrer"
              className="collection-btn-secondary"
            >
              小宇宙链接
            </a>
          </div>
        ),
        className: "editorial-page-toolbar",
      }}
    >
      {(error || readbackNotice) && (
        <p className="collection-form-error" role="alert">
          {readbackNotice || "最新清单暂时无法读取，保留上次内容。"}
          <button type="button" onClick={() => void mutate()}>
            重新尝试
          </button>
        </p>
      )}
      {data.description && (
        <p className="collection-detail-description">{data.description}</p>
      )}

      {/* 移动端操作区：工具栏右侧按钮在窄屏隐藏，这里保证可达。 */}
      <div className="collection-mobile-actions">
        <Link href="/collections" className="collection-btn-secondary">
          <IconArrowLeft aria-hidden="true" stroke={1.8} />
          <span>返回列表</span>
        </Link>
        <button
          type="button"
          className="collection-btn-secondary"
          onClick={() => setRefreshOpen(true)}
        >
          刷新清单
        </button>
        <button
          type="button"
          className="collection-btn-secondary"
          disabled={isDeleting}
          onClick={confirmDelete}
        >
          {isDeleting ? "正在删除…" : "删除清单"}
        </button>
        <a
          href={data.source_url}
          target="_blank"
          rel="noopener noreferrer"
          className="collection-btn-secondary"
        >
          小宇宙链接
        </a>
      </div>

      {refreshNotice && (
        <p className="collection-form-error" role="alert">
          {refreshNotice}
        </p>
      )}

      <RefreshCollectionModal
        isOpen={isRefreshOpen}
        collectionID={collectionID}
        onClose={() => setRefreshOpen(false)}
        onApplied={(result) => {
          setRefreshNotice(
            result.no_changes
              ? "没有变化，已记录本次检查时间。"
              : `已应用刷新，当前 ${result.item_count} 条，已收录关联保留 ${result.adopted_kept_count} 条。`,
          );
          void mutate().catch(() =>
            setReadbackNotice("刷新已保存，最新清单暂时无法读取，可重试。"),
          );
        }}
      />

      <div className="collection-toolbar">
        <div
          className="collection-status-filters"
          role="group"
          aria-label="收录状态筛选"
        >
          {ADOPTED_FILTERS.map((option) => (
            <button
              key={option.value}
              type="button"
              className={filter === option.value ? "is-active" : ""}
              aria-pressed={filter === option.value}
              onClick={() => {
                setFilter(option.value);
                const next = new URLSearchParams(params.toString());
                next.set("filter", option.value);
                router.push(`/collections/${collectionID}?${next}`, {
                  scroll: false,
                });
              }}
            >
              <span>{option.label}</span>
              <strong>
                {option.value === "all"
                  ? data.item_count
                  : option.value === "adopted"
                    ? data.adopted_count
                    : data.item_count - data.adopted_count}
              </strong>
            </button>
          ))}
        </div>
      </div>

      {visibleItems.length === 0 ? (
        <div className="collection-empty" aria-live="polite">
          <h3>当前筛选没有条目</h3>
          {failedFilterCount > 0 && (
            <p>切回“全部”查看清单中的 {items.length} 个条目。</p>
          )}
        </div>
      ) : (
        <ol className="collection-item-list">
          {visibleItems.map((item) => (
            <li
              key={item.id}
              id={`collection-item-${item.id}`}
              className="collection-item"
              data-testid="collection-item"
            >
              <span className="collection-item-index">{item.position + 1}</span>
              <div className="collection-item-main">
                <div className="collection-item-heading">
                  <div className="collection-item-cover">
                    <PodcastCover
                      coverUrl={item.image_url || item.podcast_cover_url}
                      title={item.episode_title}
                      sizes="64px"
                    />
                  </div>
                  <h3 className="collection-item-title">
                    <Link
                      href={`/collections/${collectionID}?item=${item.id}`}
                      onClick={() => setSelectedItemID(item.id)}
                    >
                      {item.episode_title}
                    </Link>
                  </h3>
                  <span
                    className={`collection-item-state ${
                      item.adopted_episode_id ? "is-adopted" : ""
                    }`}
                  >
                    {adoptedState(item)}
                  </span>
                </div>
                <p className="collection-item-meta">
                  {item.podcast_title}
                  {item.podcast_author ? ` · ${item.podcast_author}` : ""}
                  {item.duration > 0
                    ? ` · ${formatCollectionDuration(item.duration)}`
                    : ""}
                  {item.published_at
                    ? ` · ${formatCollectionDate(item.published_at)}`
                    : ""}
                  {item.pay_type && item.pay_type !== "FREE"
                    ? " · 付费单集"
                    : ""}
                  {item.is_private_media ? " · 私密音频" : ""}
                  {item.audio_available === false && !item.is_private_media
                    ? " · 暂无可用音频"
                    : ""}
                </p>
                {item.recommendation && (
                  <blockquote className="collection-item-recommendation">
                    <span className="collection-item-recommendation-label">
                      清单推荐语
                    </span>
                    <p>{item.recommendation}</p>
                  </blockquote>
                )}
                {item.shownotes && (
                  <details
                    className="collection-item-shownotes"
                    open={selectedItemID === item.id}
                  >
                    <summary
                      onClick={(event) => {
                        event.preventDefault();
                        const nextID =
                          selectedItemID === item.id ? null : item.id;
                        setSelectedItemID(nextID);
                        const next = new URLSearchParams(params.toString());
                        if (nextID) next.set("item", String(nextID));
                        else next.delete("item");
                        router.push(
                          `/collections/${collectionID}${next.size ? `?${next}` : ""}`,
                          { scroll: false },
                        );
                      }}
                    >
                      Show Notes
                    </summary>
                    <RichText
                      html={item.shownotes}
                      className="collection-item-shownotes-body"
                    />
                  </details>
                )}
                {adoptError?.itemID === item.id && (
                  <p className="collection-form-error" role="alert">
                    {adoptError.message}
                  </p>
                )}
                <div className="collection-item-actions">
                  {!item.adopted_episode_id && (
                    <button
                      type="button"
                      className="collection-btn-primary"
                      disabled={adoptingItemIDs.has(item.id)}
                      onClick={() => void adopt(item)}
                    >
                      {adoptingItemIDs.has(item.id)
                        ? "正在收录…"
                        : "加入 Inbox"}
                    </button>
                  )}
                  <a
                    href={item.episode_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="collection-btn-secondary"
                  >
                    打开原单集
                  </a>
                  {item.adopted_episode_id && (
                    <Link
                      href={`/episodes/${item.adopted_episode_id}`}
                      className="collection-btn-secondary"
                    >
                      查看收录单集
                    </Link>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}
    </PageLayout>
  );
}

export function parseCollectionID(raw: string | string[] | undefined): number {
  return positiveID(Array.isArray(raw) ? raw[0] : raw) ?? 0;
}
