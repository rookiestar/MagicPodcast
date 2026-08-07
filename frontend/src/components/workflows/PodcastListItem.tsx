import React, { memo } from "react";
import PodcastCover from "@/components/podcasts/PodcastCover";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import type { Podcast } from "@/types";

interface PodcastListItemProps {
  podcast: Podcast;
  isSelected: boolean;
  onAdd: (id: number) => void;
  onRemove: (id: number) => void;
  index: number;
}

export const PodcastListItem = memo<PodcastListItemProps>(
  ({ podcast, isSelected, onAdd, onRemove, index }) => {
    // 获取有效的封面URL（优先使用自定义封面）
    const effectiveCoverUrl = getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url);

    const handleToggle = () => {
      isSelected ? onRemove(podcast.id) : onAdd(podcast.id);
    };

    return (
      <>
        {/* === 移动端：整行可点击，绿色边框标识选中状态 === */}
        <div
          onClick={handleToggle}
          className={`
            sm:hidden flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-all mb-2
            ${isSelected
              ? "bg-green-50 dark:bg-green-900/20 border-2 border-green-500 dark:border-green-600"
              : "bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 active:bg-slate-50 dark:active:bg-slate-700"
            }
          `}
        >
          {/* 封面 - 移动端稍大 */}
          <div className="w-12 h-12 flex-shrink-0 rounded-lg overflow-hidden shadow-sm">
            <PodcastCover
              coverUrl={effectiveCoverUrl}
              title={podcast.title}
              index={index}
              priority="low"
              sizes="48px"
            />
          </div>

          {/* 标题和作者 */}
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium text-slate-900 dark:text-slate-50 truncate">
              {podcast.title}
            </div>
            {podcast.author && (
              <div className="text-xs text-slate-500 dark:text-slate-400 truncate">
                {podcast.author}
              </div>
            )}
          </div>

          {/* 选中状态图标 */}
          <div
            className={`
              w-7 h-7 flex items-center justify-center rounded-full flex-shrink-0 transition-all
              ${isSelected
                ? "bg-green-500 text-white"
                : "border-2 border-slate-300 dark:border-slate-600"
              }
            `}
          >
            {isSelected && (
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
              </svg>
            )}
          </div>
        </div>

        {/* === 桌面端：保持原有按钮式交互 === */}
        <div className="hidden sm:flex items-center justify-between p-2 hover:bg-slate-100 dark:hover:bg-slate-700 rounded cursor-pointer group mb-1">
          <div className="flex items-center gap-3 flex-1 min-w-0">
            {/* 封面 */}
            <div className="w-10 h-10 flex-shrink-0 rounded overflow-hidden shadow-sm">
              <PodcastCover
                coverUrl={effectiveCoverUrl}
                title={podcast.title}
                index={index}
                priority="low"
                sizes="40px"
              />
            </div>

            {/* 文本 */}
            <div className="flex-1 min-w-0 pr-2">
              <div className="text-xs font-medium text-slate-900 dark:text-slate-50 truncate">
                {podcast.title}
              </div>
              {podcast.author && (
                <div className="text-xs text-slate-500 dark:text-slate-400 truncate">
                  {podcast.author}
                </div>
              )}
            </div>
          </div>

          <button
            onClick={(e) => {
              e.stopPropagation();
              handleToggle();
            }}
            className={`
              w-7 h-7 flex items-center justify-center text-sm rounded flex-shrink-0 border transition-all
              ${
                isSelected
                  ? "bg-red-100 dark:bg-red-900/30 text-red-600 dark:text-red-400 border-red-200 dark:border-red-800 hover:bg-red-200 dark:hover:bg-red-900/50"
                  : "bg-blue-100 dark:bg-blue-900/30 text-blue-600 dark:text-blue-400 border-blue-200 dark:border-blue-800 hover:bg-blue-200 dark:hover:bg-blue-900/50"
              }
            `}
            title={isSelected ? "移除" : "添加"}
          >
            {isSelected ? "✕" : "✓"}
          </button>
        </div>
      </>
    );
  },
  (prevProps, nextProps) => {
    // 自定义比较函数：仅在关键属性变化时重渲染
    return (
      prevProps.podcast.id === nextProps.podcast.id &&
      prevProps.isSelected === nextProps.isSelected &&
      prevProps.podcast.title === nextProps.podcast.title
    );
  },
);

PodcastListItem.displayName = "PodcastListItem";
