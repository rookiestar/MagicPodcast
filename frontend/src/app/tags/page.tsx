"use client";

import { useEffect, useState, useMemo, useCallback, useRef, Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { tagApi, podcastApi } from "@/lib/api";
import { usePodcast, usePodcastTags } from "@/hooks/usePodcastSWR";
import { useTags } from "@/hooks/useTagSWR";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import PodcastCover from "@/components/podcasts/PodcastCover";
import TagInput from "@/components/tags/TagInput";
import TagFormModal from "@/components/tags/TagFormModal";
import PageLayout from "@/components/layout/PageLayout";
import { toast } from "@/lib/toast";
import type { Tag } from "@/types";

// 动态加载 pinyin-pro，减少首屏 bundle 大小 (~60KB)
type PinyinFunction = (text: string, options?: { pattern?: string; toneType?: string }) => string;
let pinyinModule: PinyinFunction | null = null;
const pinyinLoadPromise = import("pinyin-pro").then((mod) => {
  pinyinModule = mod.pinyin;
  return mod.pinyin;
});

type SortMode = "popularity" | "alphabetical";

interface TagsPageContentProps {
  showCreateModal: boolean;
  setShowCreateModal: React.Dispatch<React.SetStateAction<boolean>>;
  editModalTag: Tag | null;
  setEditModalTag: React.Dispatch<React.SetStateAction<Tag | null>>;
  selectedTags: Set<number>;
  setSelectedTags: React.Dispatch<React.SetStateAction<Set<number>>>;
  isSelectMode: boolean;
  sortMode: SortMode;
}

function TagsPageContent({
  showCreateModal,
  setShowCreateModal,
  editModalTag,
  setEditModalTag,
  selectedTags,
  setSelectedTags,
  isSelectMode,
  sortMode,
}: TagsPageContentProps) {
  // 拼音缓存：避免重复转换相同字符
  const pinyinCache = useRef<Map<string, string>>(new Map());
  // 跟踪 pinyin 模块是否已加载
  const [, setPinyinReady] = useState(false);

  // 预加载 pinyin 模块
  useEffect(() => {
    pinyinLoadPromise.then(() => setPinyinReady(true));
  }, []);

  const searchParams = useSearchParams();
  const podcastIdParam = searchParams.get("podcast_id");
  const podcastId = podcastIdParam ? parseInt(podcastIdParam, 10) : null;

  // 使用 SWR 获取标签列表
  const { tags, isLoading, isError, mutate } = useTags();
  const error = isError ? "加载失败" : null;

  // 使用 SWR 获取播客数据（并行请求）
  const { podcast, isLoading: podcastLoading, isError: podcastIsError } = usePodcast(podcastId);
  const { tags: podcastTags, mutate: mutatePodcastTags } = usePodcastTags(podcastId);
  const podcastError = podcastIsError ? "加载播客失败" : null;

  const handlePodcastTagsChange = async (newTags: Tag[]) => {
    if (!podcast) return;

    const currentTagIds = new Set(podcastTags.map((t) => t.id));
    const newTagIds = new Set(newTags.map((t) => t.id));

    // Find added tags
    const addedTags = newTags.filter((t) => !currentTagIds.has(t.id));
    // Find removed tags
    const removedTags = podcastTags.filter((t) => !newTagIds.has(t.id));

    try {
      // Add new tags
      for (const tag of addedTags) {
        await podcastApi.addTag(podcast.id, tag.id);
      }
      // Remove old tags
      for (const tag of removedTags) {
        await podcastApi.removeTag(podcast.id, tag.id);
      }

      // Refresh podcast tags and tag list
      mutatePodcastTags();
      mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "更新标签失败");
      // Revalidate to get correct state
      mutatePodcastTags();
    }
  };

  const handleCreateTag = async (data: { name: string; color: string }) => {
    try {
      await tagApi.create({
        name: data.name,
        color: data.color,
      });
      mutate();
    } catch (err) {
      throw err;
    }
  };

  const handleUpdateTag = async (data: { name: string; color: string }) => {
    if (!editModalTag) return;

    try {
      await tagApi.update(editModalTag.id, {
        name: data.name,
        color: data.color,
      });
      setEditModalTag(null);
      mutate();
    } catch (err) {
      throw err;
    }
  };

  const handleEditTag = (tag: Tag) => {
    setEditModalTag(tag);
  };

  const handleDeleteTag = async (id: number, name: string) => {
    if (!confirm(`确定要删除标签"${name}"吗？`)) return;

    try {
      await tagApi.delete(id);
      mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "删除失败");
    }
  };

  const handleToggleSelect = (tagId: number) => {
    const newSelected = new Set(selectedTags);
    if (newSelected.has(tagId)) {
      newSelected.delete(tagId);
    } else {
      newSelected.add(tagId);
    }
    setSelectedTags(newSelected);
  };

  // 获取中文拼音首字母（带缓存优化，动态加载 pinyin-pro）
  const getChineseInitial = useCallback((text: string): string => {
    // 处理空字符串
    if (!text || text.trim() === "") {
      return "#";
    }

    // 查询缓存
    const cached = pinyinCache.current.get(text);
    if (cached) {
      return cached;
    }

    const firstChar = text.charAt(0);

    // 如果是英文字母，直接返回大写
    if (/[a-zA-Z]/.test(firstChar)) {
      const result = firstChar.toUpperCase();
      pinyinCache.current.set(text, result);
      return result;
    }

    // 如果是汉字，使用 pinyin-pro 获取拼音首字母
    if (/\p{Script=Han}/u.test(firstChar)) {
      // 如果 pinyin 模块已加载，直接使用
      if (pinyinModule) {
        try {
          const result = pinyinModule(firstChar, {
            pattern: "first",
            toneType: "none",
          });
          const initial = result.charAt(0).toUpperCase();
          pinyinCache.current.set(text, initial);
          return initial;
        } catch (error) {
          console.warn("[getChineseInitial] pinyin conversion error:", error);
          return "Z";
        }
      }

      // 否则触发异步加载，先返回占位符
      pinyinLoadPromise.then(() => {
        // 加载完成后，清除相关缓存以便重新计算
        pinyinCache.current.delete(text);
      });
      return "Z"; // 占位符，加载完成后会重新渲染
    }

    // 其他字符（数字、符号等）归到 #
    const result = "#";
    pinyinCache.current.set(text, result);
    return result;
  }, []);

  // 按字母分组标签
  const groupedTags = useMemo(() => {
    if (sortMode === "popularity") {
      return null;
    }

    // 使用 Intl.Collator 进行中文拼音排序
    const collator = new Intl.Collator("zh-CN", { sensitivity: "base" });
    const sorted = [...tags].sort((a, b) => collator.compare(a.name, b.name));

    // 按首字母分组
    const groups: Record<string, Tag[]> = {};
    sorted.forEach((tag) => {
      // 获取拼音首字母
      const firstChar = tag.name.charAt(0);
      const letter = getChineseInitial(firstChar);

      if (!groups[letter]) {
        groups[letter] = [];
      }
      groups[letter].push(tag);
    });

    return groups;
  }, [tags, sortMode, getChineseInitial]);

  return (
    <div className="tag-content">

      {/* Podcast Preview */}
      {podcastId && (
        <div className="tag-podcast-preview">
          {podcastLoading ? (
            <div className="flex items-center justify-center py-10">
              <div className="tag-state-spinner" />
            </div>
          ) : podcastError ? (
            <div className="editorial-state is-error">
              <p>{podcastError}</p>
            </div>
          ) : podcast ? (
            <div className="flex flex-col md:flex-row gap-6">
              {/* 封面 */}
              <div className="tag-podcast-preview-cover">
                <PodcastCover
                  coverUrl={getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url)}
                  title={podcast.title}
                  priority="high"
                />
              </div>

              {/* 信息 */}
              <div className="flex-1 min-w-0">
                <h2 className="tag-podcast-preview-title">
                  {podcast.title}
                </h2>
                <p className="tag-podcast-preview-meta">
                  {podcast.author} · {podcast.episode_count} 集
                </p>
                <p className="tag-podcast-preview-desc line-clamp-2">
                  {podcast.description}
                </p>

                {/* 标签管理 */}
                <div className="tag-podcast-controls mb-4">
                  <span className="editorial-label block mb-2">
                    标签
                  </span>
                  <TagInput
                    selectedTags={podcastTags}
                    onTagsChange={handlePodcastTagsChange}
                    placeholder="点击输入框从列表选择，或输入新标签名按回车添加"
                  />
                </div>

                {/* 查看详情按钮 */}
                <Link
                  href={`/podcasts/${podcast.id}`}
                  prefetch={false}
                  className="editorial-btn editorial-btn--solid"
                >
                  查看详情 →
                </Link>
              </div>
            </div>
          ) : null}
        </div>
      )}

      {/* Error State */}
      {error && (
        <div className="editorial-state is-error">
          <h3>加载失败</h3>
          <p>{error}</p>
          <button onClick={() => mutate()} className="editorial-btn editorial-btn--danger">
            重试
          </button>
        </div>
      )}

      {/* Tags List */}
      {!error && (
        <>
          {isLoading ? (
            <div className="editorial-state">
              <div className="tag-state-spinner" />
              <p>加载中...</p>
            </div>
          ) : !isLoading && tags.length === 0 ? (
            <div className="editorial-state">
              <h3>还没有创建任何标签</h3>
              <p>为节目添加标签，便于分类与检索。</p>
              <button
                onClick={() => setShowCreateModal(true)}
                className="editorial-btn editorial-btn--primary"
              >
                创建第一个标签
              </button>
            </div>
          ) : sortMode === "alphabetical" && groupedTags ? (
            // 字母序分组显示
            <div className="space-y-6">
              {Object.keys(groupedTags)
                .sort()
                .map((letter) => (
                  <div key={letter}>
                    <h3 className="tag-group-heading">
                      {letter}
                    </h3>
                    <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-3">
                      {groupedTags[letter].map((tag) => (
                        <TagCard
                          key={tag.id}
                          tag={tag}
                          isSelectMode={isSelectMode}
                          isSelected={selectedTags.has(tag.id)}
                          onToggleSelect={() => handleToggleSelect(tag.id)}
                          onDelete={handleDeleteTag}
                          onEdit={handleEditTag}
                        />
                      ))}
                    </div>
                  </div>
                ))}
            </div>
          ) : (
            // 热度模式显示
            <div className="grid grid-cols-2 sm:grid-cols-4 md:grid-cols-6 lg:grid-cols-8 gap-3">
              {tags.map((tag) => (
                <TagCard
                  key={tag.id}
                  tag={tag}
                  isSelectMode={isSelectMode}
                  isSelected={selectedTags.has(tag.id)}
                  onToggleSelect={() => handleToggleSelect(tag.id)}
                  onDelete={handleDeleteTag}
                  onEdit={handleEditTag}
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* Create/Edit Tag Modal */}
      <TagFormModal
        isOpen={showCreateModal || editModalTag !== null}
        onClose={() => {
          setShowCreateModal(false);
          setEditModalTag(null);
        }}
        onSubmit={editModalTag ? handleUpdateTag : handleCreateTag}
        initialData={editModalTag ? { name: editModalTag.name, color: editModalTag.color } : undefined}
        mode={editModalTag ? "edit" : "create"}
      />
    </div>
  );
}

function TagCard({
  tag,
  isSelectMode,
  isSelected,
  onToggleSelect,
  onDelete,
  onEdit,
}: {
  tag: Tag;
  isSelectMode: boolean;
  isSelected: boolean;
  onToggleSelect: () => void;
  onDelete: (id: number, name: string) => void;
  onEdit: (tag: Tag) => void;
}) {
  // 根据节目数量计算视觉强度
  const count = tag.podcast_count || 0;
  const isHot = count >= 10;

  const cardClass = [
    "tag-card",
    isSelected ? "is-selected" : "",
    !isSelectMode && isHot ? "is-hot" : "",
  ]
    .filter(Boolean)
    .join(" ");

  const tagContent = (
    <>
      <span
        className="tag-card-dot"
        style={{ backgroundColor: tag.color || "#ccc" }}
      />
      <span className="tag-card-name">
        <span>{tag.name}</span>
        {tag.podcast_count !== undefined && (
          <span className="tag-card-count">({tag.podcast_count})</span>
        )}
      </span>
    </>
  );

  return (
    <div
      className={cardClass}
      onClick={isSelectMode ? onToggleSelect : undefined}
    >
      {/* 多选模式复选框 */}
      {isSelectMode && (
        <input
          type="checkbox"
          checked={isSelected}
          onChange={onToggleSelect}
          className="tag-card-checkbox"
          onClick={(e) => e.stopPropagation()}
          aria-label={`选择标签 ${tag.name}`}
        />
      )}

      {isSelectMode ? (
        <div className="tag-card-content">{tagContent}</div>
      ) : (
        <button
          type="button"
          className="tag-card-action"
          onClick={() => onEdit(tag)}
          aria-label={`编辑标签 ${tag.name}`}
        >
          {tagContent}
        </button>
      )}

      {/* 非多选模式：显示删除按钮 */}
      {!isSelectMode && (
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete(tag.id, tag.name);
          }}
          className="tag-card-delete"
          title="删除标签"
          aria-label={`删除标签 ${tag.name}`}
        >
          <svg
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M6 18L18 6M6 6l12 12"
            />
          </svg>
        </button>
      )}
    </div>
  );
}

// Wrapper component with Suspense boundary
export default function TagsPage() {
  // UI 状态管理
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [editModalTag, setEditModalTag] = useState<Tag | null>(null);
  const [selectedTags, setSelectedTags] = useState<Set<number>>(new Set());
  const [isSelectMode, setIsSelectMode] = useState(false);
  const [sortMode, setSortMode] = useState<SortMode>("popularity");

  // 使用 SWR 获取标签列表（用于工具栏显示数量）
  const { tags, isLoading: tagsLoading, mutate } = useTags();

  // 辅助函数
  const handleSelectAll = () => {
    if (selectedTags.size === tags.length) {
      setSelectedTags(new Set());
    } else {
      setSelectedTags(new Set(tags.map((t) => t.id)));
    }
  };

  const handleBatchDelete = async () => {
    if (selectedTags.size === 0) return;

    if (!confirm(`确定要删除选中的 ${selectedTags.size} 个标签吗？`)) return;

    try {
      // 逐个删除
      for (const tagId of selectedTags) {
        await tagApi.delete(tagId);
      }
      setSelectedTags(new Set());
      setIsSelectMode(false);
      // 刷新标签列表
      mutate();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "批量删除失败");
    }
  };

  return (
    <PageLayout
      rootClassName="editorial-page-shell"
      className="tag-page"
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "标签管理",
        description: !tagsLoading && tags.length > 0 ? `共 ${tags.length} 个标签` : undefined,
        rightContent: (
          <div className="flex items-center gap-2">
            {/* 新建标签按钮 - 仅在非多选模式下显示 */}
            {!isSelectMode && (
              <button
                onClick={() => setShowCreateModal(true)}
                className="editorial-btn editorial-btn--primary"
              >
                新建
              </button>
            )}

            {/* 多选/操作按钮 */}
            {isSelectMode ? (
              <>
                <button
                  onClick={() => {
                    setIsSelectMode(false);
                    setSelectedTags(new Set());
                  }}
                  className="editorial-btn editorial-btn--ghost"
                >
                  取消
                </button>
                <button
                  onClick={handleSelectAll}
                  className="editorial-btn editorial-btn--ghost"
                >
                  {selectedTags.size === tags.length ? "取消全选" : "全选"}
                </button>
                <button
                  onClick={handleBatchDelete}
                  disabled={selectedTags.size === 0}
                  className="editorial-btn editorial-btn--danger"
                >
                  删除 ({selectedTags.size})
                </button>
              </>
            ) : (
              <button
                onClick={() => setIsSelectMode(true)}
                className="editorial-btn editorial-btn--ghost"
              >
                多选
              </button>
            )}

            {/* 排序切换 */}
            <div className="editorial-segmented">
              <button
                onClick={() => setSortMode("popularity")}
                aria-pressed={sortMode === "popularity"}
              >
                热度
              </button>
              <button
                onClick={() => setSortMode("alphabetical")}
                aria-pressed={sortMode === "alphabetical"}
              >
                字母
              </button>
            </div>
          </div>
        ),
        className: "editorial-page-toolbar",
      }}
    >
      <Suspense
        fallback={
          <div className="editorial-state">
            <div className="tag-state-spinner" />
            <p>加载中...</p>
          </div>
        }
      >
        <TagsPageContent
          showCreateModal={showCreateModal}
          setShowCreateModal={setShowCreateModal}
          editModalTag={editModalTag}
          setEditModalTag={setEditModalTag}
          selectedTags={selectedTags}
          setSelectedTags={setSelectedTags}
          isSelectMode={isSelectMode}
          sortMode={sortMode}
        />
      </Suspense>
    </PageLayout>
  );
}
