"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { stripHtml, truncateText } from "@/lib/textUtils";
import { getRelativeTime, isRecentlyUpdated } from "@/lib/timeUtils";
import type { Podcast } from "@/types";
import { getProxiedImageUrl } from "@/lib/imageProxy";

interface ResponsivePodcastCardProps {
  podcast: Podcast;
  index: number;
  priority?: "high" | "medium" | "low";
  detailUrl: string;
}

/**
 * 响应式播客卡片组件
 * - 移动端：横向布局（封面左，信息右）
 * - 桌面端：保持垂直布局（优化信息密度）
 */
export function ResponsivePodcastCard({
  podcast,
  index = 0,
  priority = "medium",
  detailUrl,
}: ResponsivePodcastCardProps) {
  // 移动端检测
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 640);
    };

    checkMobile();
    window.addEventListener("resize", checkMobile);
    return () => window.removeEventListener("resize", checkMobile);
  }, []);

  // 控制简介显示
  const displayedDescription = podcast.description
    ? stripHtml(podcast.description, isMobile ? 80 : 100)
    : "";

  // 控制标签显示数量
  const displayTagCount = isMobile ? 2 : 3;
  const displayedTags = podcast.tags?.slice(0, displayTagCount) || [];
  const remainingTags = (podcast.tags?.length || 0) - displayTagCount;

  // 新更新标识
  const recentlyUpdated = isRecentlyUpdated(podcast.newest_episode_date, 7);
  const relativeTime = getRelativeTime(podcast.newest_episode_date);

  // 获取图片URL
  const imageUrl = podcast.cover_url
    ? getProxiedImageUrl(podcast.cover_url) || podcast.cover_url
    : "";

  // 优先级设置
  const isHighPriority = priority === "high" || index < 6;

  return (
    <Link href={detailUrl}>
      <div
        className={`
          bg-white rounded-xl shadow-md hover:shadow-lg
          active:scale-[0.97] active:shadow-sm
          transition-all duration-200 ease-out
          overflow-hidden cursor-pointer touch-action-manipulation
          ${isMobile ? "flex flex-row gap-3 p-3 h-auto" : "flex flex-col h-full"}
        `}
      >
        {/* 移动端：封面在左 */}
        {isMobile ? (
          <div className="w-16 h-16 flex-shrink-0 relative rounded-lg overflow-hidden bg-slate-200">
            {imageUrl ? (
              <Image
                src={imageUrl}
                alt={podcast.title}
                fill
                sizes="64px"
                className="object-cover"
                priority={isHighPriority}
                loading={isHighPriority ? "eager" : "lazy"}
              />
            ) : (
              <div className="w-full h-full flex items-center justify-center">
                <div className="text-2xl text-slate-400">🎧</div>
              </div>
            )}
          </div>
        ) : (
          // 桌面端：封面在上
          <div className="relative mx-auto w-full aspect-square">
            <div className="relative w-full h-full">
              {imageUrl ? (
                <Image
                  src={imageUrl}
                  alt={podcast.title}
                  fill
                  sizes="(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px"
                  className="object-cover"
                  priority={isHighPriority}
                  loading={isHighPriority ? "eager" : "lazy"}
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center bg-slate-200">
                  <div className="text-5xl text-slate-400">🎧</div>
                </div>
              )}
            </div>

            {/* 新更新标识 - 仅桌面端 */}
            {recentlyUpdated && (
              <div className="absolute bottom-0 right-0 m-2 z-30">
                <span className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-full bg-white text-slate-800 shadow-sm">
                  <span className="w-1.5 h-1.5 rounded-full bg-green-600" />
                  新更新
                </span>
              </div>
            )}
          </div>
        )}

        {/* 右侧内容区 */}
        <div
          className={`
            flex-1 min-w-0 flex flex-col
            ${isMobile ? "gap-1" : "p-2 md:p-4 gap-2 md:gap-3"}
          `}
        >
          {/* 标题 */}
          <h3
            className={`
              font-semibold text-slate-900 line-clamp-1 leading-tight
              ${isMobile ? "text-base" : "text-sm sm:text-base md:text-lg"}
            `}
          >
            {podcast.title}
          </h3>

          {/* 作者 - 移动端始终显示，桌面端显示 */}
          <p
            className={`
              text-slate-600 truncate
              ${isMobile ? "text-xs" : "text-xs sm:text-sm"}
            `}
          >
            {podcast.author}
          </p>

          {/* 简介 - 控制行数 */}
          {displayedDescription && (
            <p
              className={`
                text-slate-500 leading-snug
                ${isMobile ? "text-xs line-clamp-2" : "text-xs sm:text-sm line-clamp-2 md:line-clamp-3 leading-snug md:leading-relaxed"}
              `}
            >
              {displayedDescription}
            </p>
          )}

          {/* 标签 - 横向排列 */}
          {displayedTags.length > 0 && (
            <div
              className={`
                flex flex-wrap gap-1.5 mt-auto
                ${isMobile ? "mt-1" : "mt-2 md:mt-3"}
              `}
            >
              {displayedTags.map((tag) => (
                <span
                  key={tag.id}
                  className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full bg-slate-100 hover:bg-slate-200 transition-colors"
                  title={tag.name}
                >
                  <span
                    className="w-1.5 h-1.5 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tag.color }}
                  />
                  <span
                    className={isMobile ? "max-w-[60px] truncate" : "max-w-[80px] truncate"}
                  >
                    {tag.name}
                  </span>
                </span>
              ))}
              {remainingTags > 0 && (
                <span className="inline-flex items-center px-2 py-0.5 text-xs rounded-full bg-slate-100 text-slate-500">
                  +{remainingTags}
                </span>
              )}
            </div>
          )}

          {/* 桌面端底部统计信息 */}
          {!isMobile && (
            <div className="flex items-center justify-between text-xs sm:text-sm md:text-base text-slate-500 mt-auto pt-1 md:pt-3">
              <span className="font-medium">{podcast.episode_count} 集</span>
              <span className="text-[10px] sm:text-xs md:text-sm text-slate-400">
                {relativeTime}
              </span>
            </div>
          )}
        </div>
      </div>
    </Link>
  );
}
