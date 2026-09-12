"use client";

import { useMemo, useState, useEffect } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import useSWR from "swr";
import { IconPlus } from "@tabler/icons-react";
import PageLayout from "@/components/layout/PageLayout";
import PodcastCover from "@/components/podcasts/PodcastCover";
import ImportCollectionModal from "@/components/collections/ImportCollectionModal";
import {
  COLLECTIONS_PATH,
  collectionErrorMessage,
  fetchCollectionSummaries,
} from "@/lib/collections";
import type { CollectionSummary } from "@/types/collection";

const collectionListFetcher = ([, search]: readonly [string, string]) =>
  fetchCollectionSummaries(search);

/**
 * 播客清单列表：标题搜索、导入入口与真实收录计数。
 * 仅展示外部发现资料，不改变个人播客库。
 */
export default function CollectionsContent() {
  const params = useSearchParams();
  const query = params.toString();
  const [searchDraft, setSearchDraft] = useState(params.get("search") ?? "");
  const [search, setSearch] = useState(params.get("search") ?? "");
  useEffect(() => {
    const value = new URLSearchParams(query).get("search") ?? "";
    setSearch(value);
    setSearchDraft(value);
  }, [query]);
  const [isImportOpen, setImportOpen] = useState(false);
  const router = useRouter();

  const { data, error, isLoading, mutate } = useSWR<CollectionSummary[]>(
    [COLLECTIONS_PATH, search],
    collectionListFetcher,
    {
      keepPreviousData: true,
      revalidateOnFocus: false,
      shouldRetryOnError: false,
    },
  );

  const collections = useMemo(() => data ?? [], [data]);
  const failed = Boolean(error && !isLoading && collections.length === 0);

  const applySearch = (value: string) => {
    setSearchDraft(value);
    setSearch(value.trim());
    const next = new URLSearchParams(params.toString());
    if (value.trim()) next.set("search", value.trim());
    else next.delete("search");
    router.replace(`/collections${next.size ? `?${next}` : ""}`, {
      scroll: false,
    });
  };

  return (
    <PageLayout
      rootClassName="editorial-page-shell podcast-library-shell"
      className="collection-page"
      toolbar={{
        title: "播客清单",

        rightContent: (
          <button
            type="button"
            className="collection-import-button"
            onClick={() => setImportOpen(true)}
          >
            <IconPlus aria-hidden="true" stroke={1.8} />
            <span>导入清单</span>
          </button>
        ),
        className: "editorial-page-toolbar",
      }}
    >
      <div className="collection-toolbar">
        <div className="collection-search">
          <input
            type="search"
            value={searchDraft}
            onChange={(event) => applySearch(event.target.value)}
            placeholder="按清单标题搜索"
            aria-label="按清单标题搜索"
            className="collection-form-input"
          />
          {searchDraft && (
            <button
              type="button"
              className="collection-search-clear"
              onClick={() => applySearch("")}
              aria-label="清空搜索并恢复列表"
            >
              清空
            </button>
          )}
        </div>
      </div>

      {error && collections.length > 0 && (
        <p className="collection-form-error" role="alert">
          清单暂时无法刷新，保留上次内容。
          <button type="button" onClick={() => void mutate()}>
            重新尝试
          </button>
        </p>
      )}
      {isLoading && !data && <p role="status">正在读取清单…</p>}
      {failed ? (
        <div className="collection-empty" role="alert">
          <h3>清单暂时无法读取</h3>
          <p>{collectionErrorMessage(error, "连接未成功，可稍后重试。")}</p>
          <button type="button" onClick={() => void mutate()}>
            重新尝试
          </button>
        </div>
      ) : collections.length === 0 && !isLoading ? (
        <div className="collection-empty" aria-live="polite">
          {search ? (
            <>
              <h3>没有匹配的清单</h3>
              <p>换个标题关键词，或清空搜索查看全部清单。</p>
            </>
          ) : (
            <>
              <h3>还没有导入任何清单</h3>
              <p>
                粘贴一份小宇宙单集清单链接，预览后保存，就可以在这里浏览专题与逐集推荐语。
              </p>
              <button type="button" onClick={() => setImportOpen(true)}>
                导入清单
              </button>
            </>
          )}
        </div>
      ) : (
        <ul className="collection-card-list" aria-busy={isLoading}>
          {collections.map((collection) => (
            <li key={collection.id} className="collection-card">
              {!!collection.covers?.length && (
                <div className="collection-card-covers">
                  {collection.covers.map((cover, index) => (
                    <PodcastCover
                      key={`${cover}-${index}`}
                      coverUrl={cover}
                      title={`${collection.title}单集封面`}
                      index={index}
                      sizes="96px"
                    />
                  ))}
                </div>
              )}
              <div className="collection-card-main">
                <Link
                  href={`/collections/${collection.id}`}
                  className="collection-card-title"
                >
                  {collection.title}
                </Link>
                {collection.description && (
                  <p className="collection-card-description">
                    {collection.description}
                  </p>
                )}
                <p className="collection-card-meta">
                  作者：{collection.author || "未提供"}
                  <span aria-hidden="true"> · </span>
                  {collection.total_known
                    ? `共 ${collection.item_count} 集`
                    : `已读取 ${collection.item_count} 集`}
                  <span aria-hidden="true"> · </span>
                  已收录 {collection.adopted_count} 集
                  <span aria-hidden="true"> · </span>
                  来源：
                  {collection.platform === "xiaoyuzhoufm"
                    ? "小宇宙"
                    : collection.platform}
                </p>
              </div>
              <div className="collection-card-actions">
                <Link
                  href={`/collections/${collection.id}`}
                  className="collection-btn-secondary"
                >
                  打开清单
                </Link>
                <a
                  href={collection.source_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="collection-btn-secondary"
                >
                  小宇宙链接
                </a>
              </div>
            </li>
          ))}
        </ul>
      )}

      <ImportCollectionModal
        isOpen={isImportOpen}
        onClose={() => setImportOpen(false)}
        onImported={({ duplicate, collectionID }) => {
          // 新导入与重复导入都进入对应清单；重复导入打开已有清单，不建副本。
          void duplicate;
          void mutate();
          router.push(`/collections/${collectionID}`);
        }}
      />
    </PageLayout>
  );
}
