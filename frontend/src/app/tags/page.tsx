"use client";

import { useEffect, useState, useMemo, useCallback, useRef, Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { tagApi, podcastApi } from "@/lib/api";
import { usePodcast, usePodcastTags } from "@/hooks/usePodcastSWR";
import { useTags } from "@/hooks/useTagSWR";
import PodcastCover from "@/components/podcasts/PodcastCover";
import TagInput from "@/components/tags/TagInput";
import TagFormModal from "@/components/tags/TagFormModal";
import PageLayout from "@/components/layout/PageLayout";
import type { Tag, Podcast } from "@/types";

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
  setIsSelectMode: React.Dispatch<React.SetStateAction<boolean>>;
  sortMode: SortMode;
  setSortMode: React.Dispatch<React.SetStateAction<SortMode>>;
  mutateTags: () => Promise<void>;
}

function TagsPageContent({
  showCreateModal,
  setShowCreateModal,
  editModalTag,
  setEditModalTag,
  selectedTags,
  setSelectedTags,
  isSelectMode,
  setIsSelectMode,
  sortMode,
  setSortMode,
  mutateTags,
}: TagsPageContentProps) {
  // 拼音缓存：避免重复转换相同字符
  const pinyinCache = useRef<Map<string, string>>(new Map());
  // 跟踪 pinyin 模块是否已加载
  const [pinyinReady, setPinyinReady] = useState(false);

  // 预加载 pinyin 模块
  useEffect(() => {
    pinyinLoadPromise.then(() => setPinyinReady(true));
  }, []);

  const searchParams = useSearchParams();
  const podcastIdParam = searchParams.get("podcast_id");
  const podcastId = podcastIdParam ? parseInt(podcastIdParam, 10) : null;

  // 使用 SWR 获取标签列表
  const { tags, isError, mutate } = useTags();
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
      alert(err instanceof Error ? err.message : "更新标签失败");
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
      alert(err instanceof Error ? err.message : "删除失败");
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
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">

        {/* Podcast Preview */}
        {podcastId && (
          <div className="mb-8 bg-white dark:bg-slate-800 rounded-lg shadow-md p-6 border border-slate-200 dark:border-slate-700">
            {podcastLoading ? (
              <div className="flex items-center justify-center py-12">
                <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
              </div>
            ) : podcastError ? (
              <div className="bg-red-50 border border-red-200 rounded-lg p-4">
                <p className="text-red-600">{podcastError}</p>
              </div>
            ) : podcast ? (
              <div className="flex gap-6">
                {/* 封面 */}
                <div className="w-32 h-32 flex-shrink-0">
                  <PodcastCover
                    coverUrl={podcast.cover_url}
                    title={podcast.title}
                    priority="high"
                  />
                </div>

                {/* 信息 */}
                <div className="flex-1">
                  <h2 className="text-2xl font-bold text-slate-900 dark:text-slate-50 mb-2">
                    {podcast.title}
                  </h2>
                  <p className="text-sm text-slate-600 dark:text-slate-400 mb-3">
                    {podcast.author} · {podcast.episode_count} 集
                  </p>
                  <p className="text-sm text-slate-700 dark:text-slate-300 mb-4 line-clamp-2">
                    {podcast.description}
                  </p>

                  {/* 标签管理 */}
                  <div className="mb-4">
                    <span className="font-semibold text-slate-900 dark:text-slate-50 block mb-2">
                      标签：
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
                    className="inline-block px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
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
          <div className="bg-red-50 border border-red-200 rounded-lg p-6 mb-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600 mb-4">{error}</p>
            <button
              onClick={() => mutate()}
              className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700"
            >
              重试
            </button>
          </div>
        )}

        {/* Tags List */}
        {!error && (
          <>

            {tags.length === 0 ? (
              <div className="bg-white rounded-lg p-12 text-center">
                <p className="text-slate-600 mb-4">还没有创建任何标签</p>
                <button
                  onClick={() => setShowCreateModal(true)}
                  className="px-6 py-3 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
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
                      <h3 className="text-lg font-semibold text-slate-800 mb-3 sticky top-0 bg-slate-50 py-2 border-b border-slate-200">
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
    </main>
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
  let intensityClass = "";
  let countColorClass = "text-slate-400";

  if (count >= 50) {
    intensityClass = "ring-2 ring-blue-300 bg-blue-100";
    countColorClass = "text-blue-700 font-bold";
  } else if (count >= 30) {
    intensityClass = "ring-1 ring-blue-200 bg-blue-50";
    countColorClass = "text-blue-600 font-semibold";
  } else if (count >= 10) {
    intensityClass = "ring-1 ring-slate-200 bg-slate-100";
    countColorClass = "text-slate-600 font-medium";
  }

  const cardClass = [
    "rounded-lg shadow-sm px-3 py-2 h-12 flex items-center gap-2",
    "hover:shadow-md transition-all cursor-pointer relative",
    isSelected ? "ring-2 ring-blue-500" : "",
    intensityClass,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div
      className={cardClass}
      onClick={isSelectMode ? onToggleSelect : () => onEdit(tag)}
    >
      {/* 多选模式复选框 */}
      {isSelectMode && (
        <div className="absolute top-2 right-2">
          <input
            type="checkbox"
            checked={isSelected}
            onChange={onToggleSelect}
            className="w-4 h-4 rounded border-slate-300 text-blue-600 focus:ring-blue-500"
            onClick={(e) => e.stopPropagation()}
          />
        </div>
      )}

      {/* 非多选模式：显示删除按钮 */}
      {!isSelectMode && (
        <div className="absolute top-1/2 right-1 -translate-y-1/2">
          <button
            onClick={(e) => {
              e.stopPropagation();
              onDelete(tag.id, tag.name);
            }}
            className="text-slate-400 hover:text-red-600 transition-colors p-1 rounded hover:bg-red-50"
            title="删除标签"
          >
            <svg
              className="w-4 h-4"
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
        </div>
      )}

      {/* 颜色圆点 */}
      <div
        className="w-3 h-3 rounded-full flex-shrink-0"
        style={{ backgroundColor: tag.color || "#ccc" }}
      />

      {/* 标签名称和数量 */}
      <h3 className="text-sm font-normal text-slate-900 truncate flex-1">
        {tag.name}
        {tag.podcast_count !== undefined && (
          <span className={countColorClass + " ml-1"}>
            ({tag.podcast_count})
          </span>
        )}
      </h3>
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
  const { tags, mutate } = useTags();

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
      alert(err instanceof Error ? err.message : "批量删除失败");
    }
  };

  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "标签管理",
        description: tags.length > 0 ? `共 ${tags.length} 个标签` : undefined,
        rightContent: (
          <div className="flex items-center gap-2">
            {/* 新建标签按钮 - 仅在非多选模式下显示 */}
            {!isSelectMode && (
              <button
                onClick={() => setShowCreateModal(true)}
                className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm font-medium"
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
                  className="px-4 py-2 bg-white text-slate-700 rounded-lg border border-slate-300 hover:bg-slate-50 transition-colors text-sm"
                >
                  取消
                </button>
                <button
                  onClick={handleSelectAll}
                  className="px-4 py-2 bg-white text-slate-700 rounded-lg border border-slate-300 hover:bg-slate-50 transition-colors text-sm"
                >
                  {selectedTags.size === tags.length ? "取消全选" : "全选"}
                </button>
                <button
                  onClick={handleBatchDelete}
                  disabled={selectedTags.size === 0}
                  className="px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 transition-colors disabled:bg-red-300 disabled:cursor-not-allowed text-sm"
                >
                  删除 ({selectedTags.size})
                </button>
              </>
            ) : (
              <button
                onClick={() => setIsSelectMode(true)}
                className="px-4 py-2 bg-white text-slate-700 rounded-lg border border-slate-300 hover:bg-slate-50 transition-colors text-sm"
              >
                多选
              </button>
            )}

            {/* 排序切换 */}
            <div className="flex items-center gap-1">
              <button
                onClick={() => setSortMode("popularity")}
                className={
                  "px-3 py-1.5 rounded-lg text-sm transition-colors " +
                  (sortMode === "popularity"
                    ? "bg-slate-800 text-white"
                    : "bg-slate-100 text-slate-600 hover:bg-slate-200")
                }
              >
                热度
              </button>
              <button
                onClick={() => setSortMode("alphabetical")}
                className={
                  "px-3 py-1.5 rounded-lg text-sm transition-colors " +
                  (sortMode === "alphabetical"
                    ? "bg-slate-800 text-white"
                    : "bg-slate-100 text-slate-600 hover:bg-slate-200")
                }
              >
                字母
              </button>
            </div>
          </div>
        ),
      }}
    >
      <Suspense fallback={<div className="py-8 text-center text-slate-500">加载中...</div>}>
        <TagsPageContent
          showCreateModal={showCreateModal}
          setShowCreateModal={setShowCreateModal}
          editModalTag={editModalTag}
          setEditModalTag={setEditModalTag}
          selectedTags={selectedTags}
          setSelectedTags={setSelectedTags}
          isSelectMode={isSelectMode}
          setIsSelectMode={setIsSelectMode}
          sortMode={sortMode}
          setSortMode={setSortMode}
          mutateTags={mutate}
        />
      </Suspense>
    </PageLayout>
  );
}
