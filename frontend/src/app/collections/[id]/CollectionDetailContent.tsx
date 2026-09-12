"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import useSWR from "swr";
import { IconArrowLeft } from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import RichText from "@/components/RichText";
import { positiveID } from "@/lib/navigation";
import {
  ADOPTED_FILTERS,
  collectionErrorMessage,
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

  const { data, error, isLoading } = useSWR(
    collectionID > 0 ? `/api/v1/collections/${collectionID}` : null,
    () => fetchCollectionDetail(collectionID),
    {
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );

  const items = useMemo(() => data?.items ?? [], [data]);
  const visibleItems = useMemo(
    () => filterItemsByAdoptedState(items, filter),
    [items, filter],
  );

  if (collectionID === 0) {
    return (
      <PageLayout rootClassName="editorial-page-shell podcast-library-shell" className="collection-page">
        <div className="collection-empty" role="alert">
          <h3>清单地址无效</h3>
          <Link href="/collections" className="collection-btn-secondary">
            返回播客清单
          </Link>
        </div>
      </PageLayout>
    );
  }

  if (error) {
    return (
      <PageLayout rootClassName="editorial-page-shell podcast-library-shell" className="collection-page">
        <div className="collection-empty" role="alert">
          <h3>清单暂时无法读取</h3>
          <p>{collectionErrorMessage(error, "连接未成功，可稍后重试。")}</p>
          <Link href="/collections" className="collection-btn-secondary">
            返回播客清单
          </Link>
        </div>
      </PageLayout>
    );
  }

  if (isLoading || !data) {
    return (
      <PageLayout rootClassName="editorial-page-shell podcast-library-shell" className="collection-page">
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
          data.total_known ? `共 ${data.item_count} 集` : `已读取 ${data.item_count} 集`
        } · 已收录 ${data.adopted_count} 集`,
        mobileDescription: `作者：${data.author || "未提供"} · 已收录 ${data.adopted_count} 集`,
        rightContent: (
          <div className="collection-toolbar-actions">
            <Link href="/collections" className="collection-btn-secondary">
              <IconArrowLeft aria-hidden="true" stroke={1.8} />
              <span>返回列表</span>
            </Link>
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
      {data.description && (
        <p className="collection-detail-description">{data.description}</p>
      )}

      <div className="collection-toolbar">
        <div className="collection-status-filters" role="group" aria-label="收录状态筛选">
          {ADOPTED_FILTERS.map((option) => (
            <button
              key={option.value}
              type="button"
              className={filter === option.value ? "is-active" : ""}
              aria-pressed={filter === option.value}
              onClick={() => setFilter(option.value)}
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
            <li key={item.id} className="collection-item" data-testid="collection-item">
              <span className="collection-item-index">{item.position + 1}</span>
              <div className="collection-item-main">
                <div className="collection-item-heading">
                  <h3 className="collection-item-title">{item.episode_title}</h3>
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
                  {item.duration > 0 ? ` · ${formatCollectionDuration(item.duration)}` : ""}
                  {item.published_at ? ` · ${formatCollectionDate(item.published_at)}` : ""}
                  {item.pay_type && item.pay_type !== "FREE" ? " · 付费单集" : ""}
                  {item.is_private_media ? " · 私密音频" : ""}
                </p>
                {item.recommendation && (
                  <blockquote className="collection-item-recommendation">
                    <span className="collection-item-recommendation-label">清单推荐语</span>
                    <p>{item.recommendation}</p>
                  </blockquote>
                )}
                {item.shownotes && (
                  <details className="collection-item-shownotes">
                    <summary>Show Notes</summary>
                    <RichText html={item.shownotes} className="collection-item-shownotes-body" />
                  </details>
                )}
                <div className="collection-item-actions">
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
