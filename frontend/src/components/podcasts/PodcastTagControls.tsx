import TagInput from "@/components/tags/TagInput";
import type { Tag } from "@/types";

export const PODCAST_TAG_INPUT_PLACEHOLDER =
  "点击输入框从列表选择，或输入新标签名按回车添加";

interface PodcastTagControlsProps {
  tags: Tag[];
  isUpdatingTags?: boolean;
  onTagsChange: (tags: Tag[]) => void;
}

export function MobilePodcastTagControls({
  tags,
  isUpdatingTags,
  onTagsChange,
}: PodcastTagControlsProps) {
  return (
    <div className="text-sm">
      <span className="font-semibold text-slate-900">标签：</span>
      <div className="mt-2">
        {tags.length > 0 && (
          <div className="mb-2 inline-flex flex-wrap items-center gap-1.5">
            {tags.slice(0, 3).map((tag) => (
              <span
                key={tag.id}
                className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600"
              >
                <span
                  className="h-1 w-1 flex-shrink-0 rounded-full"
                  style={{ backgroundColor: tag.color }}
                />
                <span className="max-w-[100px] truncate">{tag.name}</span>
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
          placeholder={PODCAST_TAG_INPUT_PLACEHOLDER}
          showSelectedTags={false}
          disabled={isUpdatingTags}
        />
      </div>
    </div>
  );
}

export function DesktopPodcastTagControls({
  tags,
  isUpdatingTags,
  onTagsChange,
}: PodcastTagControlsProps) {
  return (
    <div>
      <div className="inline-flex flex-wrap items-center gap-2">
        <span className="font-semibold text-slate-900">标签：</span>
        {tags.length > 0 && (
          <div className="inline-flex flex-wrap items-center gap-1.5">
            {tags.map((tag) => (
              <span
                key={tag.id}
                className="group inline-flex items-center gap-1 rounded-full bg-slate-100 px-3 py-1 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-200"
              >
                <span
                  className="h-1.5 w-1.5 flex-shrink-0 rounded-full"
                  style={{ backgroundColor: tag.color }}
                />
                <span className="max-w-[120px] truncate" title={tag.name}>
                  {tag.name}
                </span>
                <button
                  type="button"
                  disabled={isUpdatingTags}
                  onClick={() => onTagsChange(tags.filter((t) => t.id !== tag.id))}
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
          placeholder={PODCAST_TAG_INPUT_PLACEHOLDER}
          showSelectedTags={false}
          disabled={isUpdatingTags}
        />
      </div>
    </div>
  );
}
