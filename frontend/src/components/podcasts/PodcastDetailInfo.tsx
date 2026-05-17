"use client";

import { useState } from "react";
import RichText from "@/components/RichText";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import type { Podcast, Tag } from "@/types";
import PodcastCover from "./PodcastCover";
import PodcastNotesEditor from "./PodcastNotesEditor";
import {
  DesktopPodcastTagControls,
  MobilePodcastTagControls,
} from "./PodcastTagControls";

interface PodcastDetailInfoProps {
  podcast: Podcast;
  tags: Tag[];
  notes: string;
  isEditingNotes: boolean;
  isSavingNotes?: boolean;
  isUpdatingTags?: boolean;
  onNotesChange: (notes: string) => void;
  onEditNotes: () => void;
  onSaveNotes: () => void;
  onCancelNotesEdit: () => void;
  onTagsChange: (tags: Tag[]) => void;
}

function formatNewestEpisodeDate(value?: string) {
  try {
    const date = value ? new Date(value) : null;
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
}

function formatDurationLabel(duration?: number) {
  if (!duration) return null;
  return `${Math.floor(duration / 60)}分${duration % 60}秒`;
}

export function MobilePodcastDetailInfo({
  podcast,
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
}: PodcastDetailInfoProps) {
  const [detailsOpen, setDetailsOpen] = useState(false);
  const coverUrl = getEffectiveCoverUrl(
    podcast.custom_cover_url,
    podcast.cover_url,
  );

  return (
    <div className="md:hidden">
      <div className="bg-white rounded-lg shadow-lg overflow-hidden p-4">
        <div className="flex gap-4 mb-4">
          <div className="w-24 h-24 flex-shrink-0">
            <div className="aspect-square w-full rounded-lg overflow-hidden">
              <PodcastCover
                coverUrl={coverUrl}
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

        <div className="flex gap-2 mb-3">
          {podcast.newest_enclosure_url && (
            <button
              type="button"
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
            type="button"
            aria-expanded={detailsOpen}
            onClick={() => setDetailsOpen((open) => !open)}
            className="flex-1 py-2 border border-slate-300 hover:bg-slate-50 rounded-lg transition-colors text-sm font-medium"
          >
            {detailsOpen ? "收起详细信息" : "展开详细信息"}
          </button>
        </div>

        <details open={detailsOpen} className="mt-4">
          <summary className="hidden"></summary>
          <div className="pt-4 border-t border-slate-200 space-y-4">
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

            {podcast.popularity_score && podcast.popularity_score >= 7 && (
              <div>
                <span className="inline-flex items-center px-3 py-1 rounded-full text-xs font-medium bg-orange-100 text-orange-800">
                  🔥 热门播客 (热度: {podcast.popularity_score}/10)
                </span>
              </div>
            )}

            <div className="text-sm">
              <span className="font-semibold text-slate-900">简介：</span>
              <div className="mt-1 line-clamp-3">
                <RichText html={podcast.description || "暂无简介"} />
              </div>
            </div>

            <MobilePodcastTagControls
              tags={tags}
              isUpdatingTags={isUpdatingTags}
              onTagsChange={onTagsChange}
            />

            <div className="text-sm">
              <PodcastNotesEditor
                notes={notes}
                isEditingNotes={isEditingNotes}
                isSavingNotes={isSavingNotes}
                textareaRows={3}
                editButtonClassName="text-xs text-blue-600 hover:text-blue-700"
                saveButtonClassName="px-3 py-1.5 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm"
                cancelButtonClassName="px-3 py-1.5 bg-slate-200 text-slate-700 rounded-lg hover:bg-slate-300 transition-colors text-sm"
                readOnlyClassName="mt-1 line-clamp-2"
                emptyClassName="text-slate-400"
                onNotesChange={onNotesChange}
                onEditNotes={onEditNotes}
                onSaveNotes={onSaveNotes}
                onCancelNotesEdit={onCancelNotesEdit}
              />
            </div>
          </div>
        </details>
      </div>
    </div>
  );
}

export function DesktopPodcastDetailInfo({
  podcast,
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
}: PodcastDetailInfoProps) {
  const coverUrl = getEffectiveCoverUrl(
    podcast.custom_cover_url,
    podcast.cover_url,
  );
  const durationLabel = formatDurationLabel(podcast.newest_enclosure_duration);

  return (
    <div className="hidden md:block">
      <div className="bg-white rounded-lg shadow-lg overflow-hidden">
        <div className="md:flex">
          <div className="md:w-1/3 p-6">
            <div className="aspect-square w-full rounded-lg overflow-hidden">
              <PodcastCover
                coverUrl={coverUrl}
                title={podcast.title}
                priority="high"
              />
            </div>
          </div>

          <div className="md:w-2/3 p-8">
            <h1 className="text-3xl font-bold text-slate-900 mb-4">
              {podcast.title}
            </h1>

            <div className="space-y-4 text-slate-600">
              <div className="flex flex-wrap gap-6 items-center">
                <div>
                  <span className="font-semibold text-slate-900">主播：</span>
                  {podcast.author}
                </div>
                <div>
                  <span className="font-semibold text-slate-900">单集数：</span>
                  {podcast.episode_count || 0}
                </div>
                <div>
                  <span className="font-semibold text-slate-900">
                    最新更新：
                  </span>
                  {formatNewestEpisodeDate(podcast.newest_episode_date)}
                </div>
                {podcast.newest_enclosure_url && (
                  <button
                    type="button"
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
                    {durationLabel && (
                      <span className="text-xs opacity-80">
                        {durationLabel}
                      </span>
                    )}
                  </button>
                )}
              </div>

              {podcast.link && (
                <div>
                  <span className="font-semibold text-slate-900">官网：</span>
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

              {podcast.popularity_score && podcast.popularity_score >= 7 && (
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

              <DesktopPodcastTagControls
                tags={tags}
                isUpdatingTags={isUpdatingTags}
                onTagsChange={onTagsChange}
              />

              <PodcastNotesEditor
                notes={notes}
                isEditingNotes={isEditingNotes}
                isSavingNotes={isSavingNotes}
                textareaRows={4}
                editButtonClassName="text-sm text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                saveButtonClassName="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
                cancelButtonClassName="px-4 py-2 bg-slate-200 text-slate-700 dark:text-slate-300 rounded-lg hover:bg-slate-300 dark:hover:bg-slate-600 transition-colors"
                readOnlyClassName="text-sm bg-slate-50/50 p-3 rounded-lg"
                emptyClassName="text-slate-400 dark:text-slate-500"
                onNotesChange={onNotesChange}
                onEditNotes={onEditNotes}
                onSaveNotes={onSaveNotes}
                onCancelNotesEdit={onCancelNotesEdit}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
