"use client";

import PrefetchLink from "@/components/common/PrefetchLink";
import PodcastCover from "@/components/podcasts/PodcastCover";
import { stripHtml } from "@/lib/textUtils";
import { getRelativeTime } from "@/lib/timeUtils";
import type { Podcast } from "@/types";

interface ResponsivePodcastCardProps {
  podcast: Podcast;
  index: number;
  priority?: "high" | "medium" | "low";
  detailUrl: string;
  isMobile: boolean;
}

export default function ResponsivePodcastCard({
  podcast,
  index = 0,
  priority = "medium",
  detailUrl,
  isMobile,
}: ResponsivePodcastCardProps) {
  // 控制简介显示
  const displayedDescription = podcast.description
    ? stripHtml(podcast.description, isMobile ? 80 : 100)
    : "";

  // 控制标签显示数量
  const displayTagCount = isMobile ? 2 : 3;
  const displayedTags = podcast.tags?.slice(0, displayTagCount) || [];
  const remainingTags = (podcast.tags?.length || 0) - displayTagCount;

  // 相对时间
  const relativeTime = getRelativeTime(podcast.newest_episode_date);

  // 判断是否最近7天有更新
  const isNew = (() => {
    if (!podcast.newest_episode_date) return false;
    const newestDate = new Date(podcast.newest_episode_date);
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
    return newestDate >= sevenDaysAgo;
  })();

  // 移动端样式类
  if (isMobile) {
    return (
      <PrefetchLink href={detailUrl} prefetchId={podcast.id} prefetchType="podcast">
        <div className="flex flex-row gap-3 p-3 bg-white rounded-xl shadow-md hover:shadow-lg active:scale-[0.97] active:shadow-sm transition-all duration-200 ease-out overflow-hidden cursor-pointer">
          {/* 封面 */}
          <div className="w-16 h-16 flex-shrink-0 relative rounded-lg overflow-hidden">
            <PodcastCover
              coverUrl={podcast.cover_url}
              title={podcast.title}
              index={index}
              priority={priority}
              sizes="64px"
            />
            {/* 新更新标识 */}
            {isNew && (
              <div className="absolute top-0.5 right-0.5 px-0.5 py-0.5 bg-emerald-500/65 text-white text-[6px] font-bold uppercase tracking-wider rounded shadow-sm">
                NEW
              </div>
            )}
          </div>

          {/* 内容 */}
          <div className="flex-1 min-w-0 flex flex-col gap-2">
            {/* 标题 */}
            <h3 className="text-base font-semibold text-slate-900 line-clamp-1 leading-tight">
              {podcast.title}
            </h3>

            {/* 作者 */}
            <p className="text-xs text-slate-600 mb-0.5">
              {podcast.author}
            </p>

            {/* 简介 */}
            {displayedDescription && (
              <p className="text-xs text-slate-500 leading-snug line-clamp-2">
                {displayedDescription}
              </p>
            )}

            {/* 标签 */}
            {displayedTags.length > 0 && (
              <div className="flex flex-wrap gap-1.5 mt-auto">
                {displayedTags.map((tag) => (
                  <span
                    key={tag.id}
                    className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 hover:bg-slate-200"
                    title={tag.name}
                  >
                    <span
                      className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                      style={{ backgroundColor: tag.color }}
                    />
                    <span className="max-w-[60px] truncate">{tag.name}</span>
                  </span>
                ))}
                {remainingTags > 0 && (
                  <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 text-slate-500">
                    +{remainingTags}
                  </span>
                )}
              </div>
            )}
          </div>
        </div>
      </PrefetchLink>
    );
  }

  // 桌面端样式类
  return (
    <PrefetchLink href={detailUrl} prefetchId={podcast.id} prefetchType="podcast">
      <div className="flex flex-col h-full bg-white rounded-xl shadow-md hover:shadow-lg active:scale-[0.97] active:shadow-sm transition-all duration-200 ease-out overflow-hidden cursor-pointer">
        {/* 封面 */}
        <div className="relative w-full pt-[100%] rounded-lg overflow-hidden bg-slate-200">
          <PodcastCover
            coverUrl={podcast.cover_url}
            title={podcast.title}
            index={index}
            priority={priority}
            sizes="(max-width: 640px) 50vw, (max-width: 828px) 144px"
            className="!absolute !inset-0 !aspect-none"
          />
          {/* 新更新标识 */}
          {isNew && (
            <div className="absolute top-2 right-2 px-1.5 py-0.5 bg-emerald-500/65 text-white text-[10px] font-bold uppercase tracking-wider rounded-md shadow-sm">
              NEW
            </div>
          )}
        </div>

        {/* 内容 */}
        <div className="flex-1 gap-2 md:gap-3 p-4 flex flex-col">
          {/* 标题 */}
          <h3 className="text-lg font-semibold text-slate-900 line-clamp-1 leading-tight">
            {podcast.title}
          </h3>

          {/* 作者 */}
          <p className="text-xs sm:text-sm text-slate-600 mb-0.5">
            {podcast.author}
          </p>

          {/* 简介 */}
          {displayedDescription && (
            <p className="text-sm text-slate-500 leading-snug line-clamp-2 md:line-clamp-3">
              {displayedDescription}
            </p>
          )}

          {/* 标签 */}
          {displayedTags.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mt-auto">
              {displayedTags.map((tag) => (
                <span
                  key={tag.id}
                  className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 hover:bg-slate-200"
                  title={tag.name}
                >
                  <span
                    className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tag.color }}
                  />
                  <span className="max-w-[60px] truncate">{tag.name}</span>
                </span>
              ))}
              {remainingTags > 0 && (
                <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 text-slate-500">
                  +{remainingTags}
                </span>
              )}
            </div>
          )}

          {/* 底部信息 */}
          <div className="flex items-center justify-between text-xs text-slate-500 mt-auto pt-1 md:pt-3">
            <span className="font-medium">{podcast.episode_count} 集</span>
            <span className="text-[10px] sm:text-xs md:text-sm text-slate-400">
              {relativeTime}
            </span>
          </div>
        </div>
      </div>
    </PrefetchLink>
  );
}
