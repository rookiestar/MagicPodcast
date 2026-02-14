"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { useParams, useSearchParams } from "next/navigation";
import Link from "next/link";
import { podcastApi, episodeApi } from "@/lib/api";
import { usePodcast, usePodcastTags, usePodcastNotes } from "@/hooks/usePodcastSWR";
import type { Podcast, Tag, Episode } from "@/types";
import TagInput from "@/components/tags/TagInput";
import RichText from "@/components/RichText";
import EpisodeCard from "@/components/episodes/EpisodeCard";
import PodcastCover from "@/components/podcasts/PodcastCover";
import PageLayout from "@/components/layout/PageLayout";
import { PodcastDetailSkeleton } from "@/components/ui/Skeleton";
import { useBreakpoint } from "@/hooks/useBreakpoint";

export default function PodcastDetailPage() {
  const params = useParams();
  const searchParams = useSearchParams();
  const id = parseInt(params.id as string);
  const targetEpisodeId = searchParams.get("episode_id"); // 获取目标单集 ID
  const sortBy = searchParams.get("sort_by") || ""; // 获取排序方式
  const tagIds = searchParams.get("tag_ids"); // 获取标签筛选（逗号分隔）
  const episodeListRef = useRef<HTMLDivElement>(null);
  const { isMobile } = useBreakpoint();

  // 构建返回 URL 的查询参数
  const buildBackUrl = () => {
    const params = new URLSearchParams();
    if (sortBy) {
      params.append("sort_by", sortBy);
    }
    if (tagIds) {
      // tag_ids 是逗号分隔的字符串，需要转换为多个 tag_id 参数
      tagIds.split(",").forEach((id) => {
        params.append("tag_id", id);
      });
    }
    const queryString = params.toString();
    return `/podcasts${queryString ? `?${queryString}` : ""}`;
  };

  // 使用 SWR Hooks（并行请求）
  const { podcast, isLoading: podcastLoading, isError: podcastError, mutate: mutatePodcast } = usePodcast(id);
  const { tags, isLoading: tagsLoading, mutate: mutateTags } = usePodcastTags(id);
  const { notes: swrNotes, isLoading: notesLoading, mutate: mutateNotes } = usePodcastNotes(id);

  // 本地状态
  const [notes, setNotes] = useState("");
  const [isEditingNotes, setIsEditingNotes] = useState(false);
  const [episodes, setEpisodes] = useState<Episode[]>([]);
  const [episodesLoading, setEpisodesLoading] = useState(true);

  // 同步 SWR notes 到本地状态
  useEffect(() => {
    if (swrNotes !== undefined) {
      setNotes(swrNotes);
    }
  }, [swrNotes]);

  // 综合加载状态
  const loading = podcastLoading || tagsLoading || notesLoading;
  const error = podcastError ? "加载播客失败" : null;

  // 分页状态
  const [currentPage, setCurrentPage] = useState(1);
  const [hasMoreEpisodes, setHasMoreEpisodes] = useState(true);
  const [totalEpisodes, setTotalEpisodes] = useState(0);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const PAGE_SIZE = 20;

  // 单集获取函数（保留原逻辑，支持无限滚动）
  const fetchEpisodes = async (page: number = 1, append: boolean = false) => {
    try {
      if (page === 1) {
        setEpisodesLoading(true);
      } else {
        setIsLoadingMore(true);
      }

      const { episodes: newEpisodes, pagination } = await episodeApi.listByPodcast(
        id,
        page,
        PAGE_SIZE,
      );

      if (append) {
        // 追加到现有列表
        setEpisodes((prev) => [...prev, ...newEpisodes]);
      } else {
        // 首次加载，替换列表
        setEpisodes(newEpisodes);
      }

      setCurrentPage(pagination.page);
      setTotalEpisodes(pagination.total);
      setHasMoreEpisodes(pagination.has_more);
    } catch (err) {
      console.error("Failed to fetch episodes:", err);
      if (!append) {
        setEpisodes([]);
      }
    } finally {
      setEpisodesLoading(false);
      setIsLoadingMore(false);
    }
  };

  // 加载更多单集（真正的分页加载）
  const loadMoreEpisodes = useCallback(() => {
    if (isLoadingMore || !hasMoreEpisodes) return;
    fetchEpisodes(currentPage + 1, true);
  }, [isLoadingMore, hasMoreEpisodes, currentPage, id]);

  // 单集首次加载
  useEffect(() => {
    if (id && !podcastLoading) {
      fetchEpisodes();
    }
  }, [id, podcastLoading]);

  // 滚动监听 - 自动加载更多单集
  useEffect(() => {
    const handleScroll = () => {
      // 当滚动到距离底部500px时，自动加载更多
      const scrollPosition = window.innerHeight + window.scrollY;
      const threshold = document.body.offsetHeight - 500;

      if (scrollPosition >= threshold && !isLoadingMore && hasMoreEpisodes) {
        loadMoreEpisodes();
      }
    };

    window.addEventListener("scroll", handleScroll, { passive: true });
    return () => window.removeEventListener("scroll", handleScroll);
  }, [isLoadingMore, hasMoreEpisodes, loadMoreEpisodes]);

  // 当单集列表加载完成且有目标单集时，滚动到指定位置
  useEffect(() => {
    if (!episodesLoading && targetEpisodeId && episodes.length > 0) {
      const targetEpisodeIdNum = parseInt(targetEpisodeId);

      // 找到目标单集在列表中的索引
      const targetIndex = episodes.findIndex(
        (ep) => ep.id === targetEpisodeIdNum,
      );

      if (targetIndex !== -1) {
        // 等待 DOM 更新后滚动
        setTimeout(() => {
          const element = document.getElementById(`episode-${targetEpisodeId}`);
          if (element) {
            element.scrollIntoView({ behavior: "smooth", block: "center" });
            // 添加高亮效果
            element.classList.add("ring-2", "ring-blue-500", "ring-offset-2");
            setTimeout(() => {
              element.classList.remove(
                "ring-2",
                "ring-blue-500",
                "ring-offset-2",
              );
            }, 2000);
          }
        }, 300);
      }
    }
  }, [episodesLoading, targetEpisodeId, episodes]);

  // 处理标签变化（添加、移除、批量更新）
  const handleTagsChange = async (newTags: Tag[]) => {
    // 计算差异
    const currentIds = new Set(tags.map((t) => t.id));
    const newIds = new Set(newTags.map((t) => t.id));

    // 找出需要添加的标签
    const toAdd = newTags.filter((t) => !currentIds.has(t.id));
    // 找出需要移除的标签
    const toRemove = tags.filter((t) => !newIds.has(t.id));

    // 乐观更新：立即更新 UI
    mutateTags(newTags, false);

    try {
      // 先添加新标签
      for (const tag of toAdd) {
        await podcastApi.addTag(id, tag.id);
      }

      // 再移除旧标签
      for (const tag of toRemove) {
        await podcastApi.removeTag(id, tag.id);
      }

      // 重新验证数据
      mutateTags();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : "更新标签失败";
      alert(`标签更新失败: ${errorMsg}`);
      console.error("Failed to update tags:", err);
      // 回滚：重新验证恢复正确状态
      mutateTags();
    }
  };

  const handleNotesSave = async () => {
    // 乐观更新
    mutateNotes({ id, notes }, false);
    setIsEditingNotes(false);

    try {
      await podcastApi.updateNotes(id, notes);
      // 重新验证
      mutateNotes();
    } catch (err) {
      const errorMsg = err instanceof Error ? err.message : "保存备注失败";
      alert(`保存失败: ${errorMsg}`);
      console.error("Failed to save notes:", err);
      // 回滚
      mutateNotes();
      setIsEditingNotes(true);
    }
  };

  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回列表", href: buildBackUrl() }],
        title: podcast?.title || "播客详情",
        description: !loading && podcast && (podcast.episode_count || episodes.length) > 0 ? `共 ${podcast.episode_count || episodes.length} 个单集` : undefined,
      }}
    >
      <div className="py-6">

        {/* Loading State - Skeleton */}
        {loading && (
          <PodcastDetailSkeleton isMobile={isMobile} />
        )}

        {/* Error State */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-6">
            <h3 className="text-red-800 font-semibold mb-2">加载失败</h3>
            <p className="text-red-600">{error}</p>
          </div>
        )}

        {/* Podcast Detail */}
        {!loading && !error && podcast && (
          <>
            {/* 移动端：折叠式元信息 */}
            <div className="md:hidden">
              <div className="bg-white rounded-lg shadow-lg overflow-hidden p-4">
                {/* 封面 + 标题（一行显示） */}
                <div className="flex gap-4 mb-4">
                  <div className="w-24 h-24 flex-shrink-0">
                    <div className="aspect-square w-full rounded-lg overflow-hidden">
                      <PodcastCover
                        coverUrl={podcast.cover_url}
                        title={podcast.title}
                        priority="high"
                      />
                    </div>
                  </div>
                  <div className="flex-1 min-w-0">
                    <h1 className="text-xl font-bold text-slate-900 truncate">
                      {podcast.title}
                    </h1>
                    <div className="text-sm text-slate-600 mt-1">
                      <span>{podcast.author}</span>
                      <span className="mx-2">·</span>
                      <span>{podcast.episode_count || 0}集</span>
                    </div>
                  </div>
                </div>

                {/* 播放按钮 + 展开按钮 */}
                <div className="flex gap-2 mb-3">
                  {podcast.newest_enclosure_url && (
                    <button
                      onClick={() =>
                        window.open(podcast.newest_enclosure_url, "_blank")
                      }
                      className="flex-1 py-2 bg-slate-700 hover:bg-slate-800 text-white rounded-lg transition-colors text-sm font-medium flex items-center justify-center gap-1.5"
                    >
                      <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
                        <path d="M8 5v14l11-7z" />
                      </svg>
                      播放最新一集
                    </button>
                  )}
                  <button
                    onClick={() => {
                      const details = document.getElementById('podcast-details');
                      if (details) {
                        details.open = !details.open;
                      }
                    }}
                    className="flex-1 py-2 border border-slate-300 hover:bg-slate-50 rounded-lg transition-colors text-sm font-medium"
                  >
                    展开详细信息 ▼
                  </button>
                </div>

                {/* 折叠详细信息 */}
                <details
                  id="podcast-details"
                  className="mt-4"
                >
                  <summary className="hidden"></summary>
                  <div className="pt-4 border-t border-slate-200 space-y-4">
                    {/* 官网 */}
                    {podcast.link && (
                      <div className="text-sm">
                        <span className="font-semibold text-slate-900">官网：</span>
                        <a
                          href={podcast.link}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-blue-600 hover:text-blue-700 ml-2"
                        >
                          访问网站 →
                        </a>
                      </div>
                    )}

                    {/* 热门标签 */}
                    {podcast.popularity_score &&
                      podcast.popularity_score >= 7 && (
                        <div>
                          <span className="inline-flex items-center px-3 py-1 rounded-full text-xs font-medium bg-orange-100 text-orange-800">
                            🔥 热门播客 (热度: {podcast.popularity_score}/10)
                          </span>
                        </div>
                      )}

                    {/* 简介（限制3行） */}
                    <div className="text-sm">
                      <span className="font-semibold text-slate-900">简介：</span>
                      <div className="mt-1 line-clamp-3">
                        <RichText html={podcast.description || "暂无简介"} />
                      </div>
                    </div>

                    {/* 标签管理（限制显示前3个） */}
                    <div className="text-sm">
                      <span className="font-semibold text-slate-900">标签：</span>
                      <div className="mt-2">
                        {tags.length > 0 && (
                          <div className="inline-flex flex-wrap items-center gap-1.5 mb-2">
                            {tags.slice(0, 3).map((tag) => (
                              <span
                                key={tag.id}
                                className="inline-flex items-center gap-1 rounded-full font-medium text-xs px-2 py-0.5 bg-slate-100 text-slate-600"
                              >
                                <span
                                  className="w-1 h-1 rounded-full flex-shrink-0"
                                  style={{ backgroundColor: tag.color }}
                                />
                                <span className="max-w-[100px] truncate">
                                  {tag.name}
                                </span>
                              </span>
                            ))}
                            {tags.length > 3 && (
                              <span className="text-xs text-slate-500">
                                +{tags.length - 3}
                              </span>
                            )}
                          </div>
                        )}
                        <TagInput
                          selectedTags={tags}
                          onTagsChange={handleTagsChange}
                          placeholder="点击输入框从列表选择，或输入新标签名按回车添加"
                          showSelectedTags={false}
                        />
                      </div>
                    </div>

                    {/* 备注编辑 */}
                    <div className="text-sm">
                      <div className="flex items-center justify-between mb-2">
                        <span className="font-semibold text-slate-900">备注：</span>
                        {!isEditingNotes && (
                          <button
                            onClick={() => setIsEditingNotes(true)}
                            className="text-xs text-blue-600 hover:text-blue-700"
                          >
                            编辑
                          </button>
                        )}
                      </div>
                      {isEditingNotes ? (
                        <div className="space-y-2">
                          <textarea
                            value={notes}
                            onChange={(e) => setNotes(e.target.value)}
                            className="w-full px-3 py-2 border border-slate-300 rounded-lg bg-white text-sm text-slate-900 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                            rows={3}
                            placeholder="添加备注..."
                          />
                          <div className="flex gap-2">
                            <button
                              onClick={handleNotesSave}
                              className="px-3 py-1.5 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm"
                            >
                              保存
                            </button>
                            <button
                              onClick={() => {
                                setIsEditingNotes(false);
                                setNotes(swrNotes || ""); // 恢复原始内容
                              }}
                              className="px-3 py-1.5 bg-slate-200 text-slate-700 rounded-lg hover:bg-slate-300 transition-colors text-sm"
                            >
                              取消
                            </button>
                          </div>
                        </div>
                      ) : (
                        <p className="mt-1 line-clamp-2">
                          {notes || (
                            <span className="text-slate-400">暂无备注</span>
                          )}
                        </p>
                      )}
                    </div>
                  </div>
                </details>
              </div>
            </div>

            {/* 桌面端：完整布局 */}
            <div className="hidden md:block">
              <div className="bg-white rounded-lg shadow-lg overflow-hidden">
                {/* Cover */}
                <div className="md:flex">
              <div className="md:w-1/3 p-6">
                <div className="aspect-square w-full rounded-lg overflow-hidden">
                  <PodcastCover
                    coverUrl={podcast.cover_url}
                    title={podcast.title}
                    priority="high"
                  />
                </div>
              </div>

              {/* Info */}
              <div className="md:w-2/3 p-8">
                <h1 className="text-3xl font-bold text-slate-900 mb-4">
                  {podcast.title}
                </h1>

                <div className="space-y-4 text-slate-600">
                  {/* 主播信息、单集数、最新更新、播放按钮 - 合并为同一行 */}
                  <div className="flex flex-wrap gap-6 items-center">
                    <div>
                      <span className="font-semibold text-slate-900">
                        主播：
                      </span>
                      {podcast.author}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900">
                        单集数：
                      </span>
                      {podcast.episode_count || 0}
                    </div>
                    <div>
                      <span className="font-semibold text-slate-900">
                        最新更新：
                      </span>
                      {(() => {
                        try {
                          const date = podcast.newest_episode_date
                            ? new Date(podcast.newest_episode_date)
                            : null;
                          return date && !isNaN(date.getTime())
                            ? date.toLocaleString("zh-CN", {
                                year: "numeric",
                                month: "2-digit",
                                day: "2-digit",
                                hour: "2-digit",
                                minute: "2-digit",
                                second: "2-digit",
                                hour12: false,
                              })
                            : "未知";
                        } catch {
                          return "未知";
                        }
                      })()}
                    </div>
                    {/* 🆕 最新单集播放按钮 */}
                    {podcast.newest_enclosure_url && (
                      <button
                        onClick={() =>
                          window.open(podcast.newest_enclosure_url, "_blank")
                        }
                        className="ml-2 px-2.5 py-1.5 bg-slate-700 hover:bg-slate-800 text-white text-sm rounded-lg transition-colors inline-flex items-center gap-1.5"
                        title="播放最新一集"
                      >
                        <svg
                          className="w-4 h-4"
                          fill="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path d="M8 5v14l11-7z" />
                        </svg>
                        {podcast.newest_enclosure_duration && (
                          <span className="text-xs opacity-80">
                            {Math.floor(podcast.newest_enclosure_duration / 60)}
                            分{podcast.newest_enclosure_duration % 60}秒
                          </span>
                        )}
                      </button>
                    )}
                  </div>

                  {/* 🆕 播客官网链接 */}
                  {podcast.link && (
                    <div>
                      <span className="font-semibold text-slate-900">
                        官网：
                      </span>
                      <a
                        href={podcast.link}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 ml-2"
                      >
                        访问网站 →
                      </a>
                    </div>
                  )}

                  {/* 🆕 热门标签 */}
                  {podcast.popularity_score &&
                    podcast.popularity_score >= 7 && (
                      <div>
                        <span className="inline-flex items-center px-3 py-1 rounded-full text-sm font-medium bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200">
                          🔥 热门播客 (热度: {podcast.popularity_score}/10)
                        </span>
                      </div>
                    )}

                  <div>
                    <span className="font-semibold text-slate-900">简介：</span>
                    <div className="mt-1 text-sm">
                      <RichText html={podcast.description || "暂无简介"} />
                    </div>
                  </div>

                  {/* 标签管理 */}
                  <div>
                    <div className="inline-flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-slate-900">
                        标签：
                      </span>
                      {/* 已选标签 - 紧跟"标签："后面 */}
                      {tags.length > 0 && (
                        <div className="inline-flex flex-wrap items-center gap-1.5">
                          {tags.map((tag) => (
                            <span
                              key={tag.id}
                              className="inline-flex items-center gap-1 rounded-full font-medium text-sm px-3 py-1 bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors group"
                            >
                              <span
                                className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                                style={{ backgroundColor: tag.color }}
                              />
                              <span
                                className="max-w-[120px] truncate"
                                title={tag.name}
                              >
                                {tag.name}
                              </span>
                              <button
                                onClick={() =>
                                  handleTagsChange(
                                    tags.filter((t) => t.id !== tag.id),
                                  )
                                }
                                className="ml-0.5 opacity-0 group-hover:opacity-100 hover:text-red-600 dark:hover:text-red-400 transition-opacity"
                                title="删除标签"
                              >
                                ✕
                              </button>
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                    {/* 标签输入框 - 换行显示 */}
                    <div className="mt-3">
                      <TagInput
                        selectedTags={tags}
                        onTagsChange={handleTagsChange}
                        placeholder="点击输入框从列表选择，或输入新标签名按回车添加"
                        showSelectedTags={false}
                      />
                    </div>
                  </div>

                  {/* 备注编辑 */}
                  <div>
                    <div className="flex items-center justify-between mb-2">
                      <span className="font-semibold text-slate-900">
                        备注：
                      </span>
                      {!isEditingNotes && (
                        <button
                          onClick={() => setIsEditingNotes(true)}
                          className="text-sm text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                        >
                          编辑
                        </button>
                      )}
                    </div>
                    {isEditingNotes ? (
                      <div className="space-y-2">
                        <textarea
                          value={notes}
                          onChange={(e) => setNotes(e.target.value)}
                          className="w-full px-3 py-2 border border-slate-300 rounded-lg bg-white text-sm text-slate-900 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                          rows={4}
                          placeholder="添加备注..."
                        />
                        <div className="flex gap-2">
                          <button
                            onClick={handleNotesSave}
                            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
                          >
                            保存
                          </button>
                          <button
                            onClick={() => {
                              setIsEditingNotes(false);
                              setNotes(swrNotes || ""); // 恢复原始内容
                            }}
                            className="px-4 py-2 bg-slate-200 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-300 dark:hover:bg-slate-600 transition-colors"
                          >
                            取消
                          </button>
                        </div>
                      </div>
                    ) : (
                      <p className="text-sm bg-slate-50/50 p-3 rounded-lg">
                        {notes || (
                          <span className="text-slate-400 dark:text-slate-500">
                            暂无备注
                          </span>
                        )}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
            </div>
          </>
        )}

        {/* Episodes List - 新增section */}
        {!loading && !error && podcast && (
          <div className="mt-8" ref={episodeListRef}>
            <h2 className="text-2xl font-bold text-slate-900 mb-6">
              单集列表 ({totalEpisodes > 0 ? totalEpisodes : episodes.length} 集)
            </h2>

            {/* 初始加载状态 - 显示骨架屏 */}
            {episodesLoading && episodes.length === 0 ? (
              <div className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  {[1, 2, 3, 4].map((i) => (
                    <div
                      key={i}
                      className="bg-white rounded-lg shadow-sm p-6 animate-pulse"
                    >
                      <div className="flex items-start gap-3">
                        <div className="w-16 h-16 bg-slate-200 rounded-lg"></div>
                        <div className="flex-1 space-y-2">
                          <div className="h-4 bg-slate-200 rounded w-3/4"></div>
                          <div className="h-3 bg-slate-200 rounded w-1/2"></div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
                <p className="text-center text-sm text-slate-600 mt-6">
                  正在加载单集列表...
                </p>
              </div>
            ) : episodes.length === 0 ? (
              <div className="bg-white rounded-lg p-12 text-center shadow-sm">
                <div className="text-6xl mb-4">📭</div>
                <p className="text-slate-600 text-lg">暂无单集</p>
                <p className="text-slate-5000 text-sm mt-2">
                  点击下方按钮同步单集数据
                </p>
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                  {episodes.map((episode, index) => (
                    <div
                      key={episode.id}
                      id={`episode-${episode.id}`}
                      className="transition-all duration-200"
                    >
                      <EpisodeCard
                        episode={episode}
                        podcastCover={podcast.cover_url}
                        index={index}
                        priority={
                          index < 3 ? "high" : index < 10 ? "medium" : "low"
                        }
                      />
                    </div>
                  ))}
                </div>

                {/* 加载更多按钮 */}
                {hasMoreEpisodes && !isLoadingMore && (
                  <div className="text-center mt-8">
                    <button
                      onClick={loadMoreEpisodes}
                      className="px-6 py-3 bg-white text-slate-800 font-medium rounded-xl border border-slate-300 hover:bg-slate-50 hover:border-slate-400 transition-colors"
                    >
                      加载更多 ({episodes.length}/{totalEpisodes})
                    </button>
                  </div>
                )}

                {/* 加载更多提示 */}
                {isLoadingMore && (
                  <div className="text-center mt-8">
                    <p className="text-sm text-slate-600 flex items-center justify-center gap-2">
                      <span className="inline-block animate-spin rounded-full h-4 w-4 border-b-2 border-blue-600"></span>
                      正在加载更多单集...
                    </p>
                  </div>
                )}

                {/* 全部加载完成提示 */}
                {!hasMoreEpisodes && episodes.length > 0 && (
                  <div className="text-center mt-8 text-sm text-slate-500">
                    已加载全部 {episodes.length} 集单集
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </PageLayout>
  );
}
