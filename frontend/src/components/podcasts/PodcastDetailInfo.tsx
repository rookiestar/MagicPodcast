"use client";

import { useState } from "react";
import RichText from "@/components/RichText";
import TagInput from "@/components/tags/TagInput";
import { getEffectiveCoverUrl } from "@/lib/imageProxy";
import type { Podcast, Tag } from "@/types";
import PodcastCover from "./PodcastCover";

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

const TAG_INPUT_PLACEHOLDER = "点击输入框从列表选择，或输入新标签名按回车添加";

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

function PodcastNotesEditor({
  notes,
  isEditingNotes,
  isSavingNotes = false,
  textareaRows,
  editButtonClassName,
  saveButtonClassName,
  cancelButtonClassName,
  readOnlyClassName,
  emptyClassName,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
}: {
  notes: string;
  isEditingNotes: boolean;
  isSavingNotes?: boolean;
  textareaRows: number;
  editButtonClassName: string;
  saveButtonClassName: string;
  cancelButtonClassName: string;
  readOnlyClassName: string;
  emptyClassName: string;
  onNotesChange: (notes: string) => void;
  onEditNotes: () => void;
  onSaveNotes: () => void;
  onCancelNotesEdit: () => void;
}) {
  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="font-semibold text-slate-900">备注：</span>
        {!isEditingNotes && (
          <button
            type="button"
            onClick={onEditNotes}
            className={editButtonClassName}
          >
            编辑
          </button>
        )}
      </div>
      {isEditingNotes ? (
        <div className="space-y-2">
          <textarea
            value={notes}
            onChange={(event) => onNotesChange(event.target.value)}
            disabled={isSavingNotes}
            className="w-full px-3 py-2 border border-slate-300 rounded-lg bg-white text-sm text-slate-900 focus:ring-2 focus:ring-blue-500 focus:border-transparent"
            rows={textareaRows}
            placeholder="添加备注..."
          />
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onSaveNotes}
              disabled={isSavingNotes}
              className={`${saveButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              {isSavingNotes ? "保存中..." : "保存"}
            </button>
            <button
              type="button"
              onClick={onCancelNotesEdit}
              disabled={isSavingNotes}
              className={`${cancelButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              取消
            </button>
          </div>
        </div>
      ) : (
        <p className={readOnlyClassName}>
          {notes || <span className={emptyClassName}>暂无备注</span>}
        </p>
      )}
    </div>
  );
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
                  onTagsChange={onTagsChange}
                  placeholder={TAG_INPUT_PLACEHOLDER}
                  showSelectedTags={false}
                  disabled={isUpdatingTags}
                />
              </div>
            </div>

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

              <div>
                <div className="inline-flex flex-wrap items-center gap-2">
                  <span className="font-semibold text-slate-900">标签：</span>
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
                            type="button"
                            disabled={isUpdatingTags}
                            onClick={() =>
                              onTagsChange(tags.filter((t) => t.id !== tag.id))
                            }
                            className="ml-0.5 opacity-0 transition-opacity hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-30 group-hover:opacity-100 dark:hover:text-red-400"
                            title="删除标签"
                          >
                            ✕
                          </button>
                        </span>
                      ))}
                    </div>
                  )}
                </div>
                <div className="mt-3">
                  <TagInput
                    selectedTags={tags}
                    onTagsChange={onTagsChange}
                    placeholder={TAG_INPUT_PLACEHOLDER}
                    showSelectedTags={false}
                    disabled={isUpdatingTags}
                  />
                </div>
              </div>

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
