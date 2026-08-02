"use client";

import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import type { Tag } from "@/types";

interface PodcastTagFilterProps {
  displayTags: Tag[];
  selectedTagIds: number[];
  hasMoreTags: boolean;
  showAllTags: boolean;
  onTagToggle: (tagId: number | null) => void;
  onShowAllTagsChange: (showAllTags: boolean) => void;
}

export default function PodcastTagFilter({
  displayTags,
  selectedTagIds,
  hasMoreTags,
  showAllTags,
  onTagToggle,
  onShowAllTagsChange,
}: PodcastTagFilterProps) {
  if (displayTags.length === 0) {
    return null;
  }

  return (
    <section className="podcast-library-filters" aria-label="播客库筛选">
      <div className="podcast-library-filter-list">
        <button
          onClick={() => onTagToggle(null)}
          aria-pressed={selectedTagIds.length === 0}
        >
          全部
        </button>

        {displayTags.map((tag) => {
          const isSelected = selectedTagIds.includes(tag.id);

          return (
            <button
              key={tag.id}
              onClick={() => onTagToggle(tag.id)}
              title={tag.name}
              aria-pressed={isSelected}
            >
              <span
                className="podcast-library-filter-dot"
                style={{
                  backgroundColor: isSelected ? "#ffffff" : tag.color,
                }}
              />
              <span className="max-w-[100px] truncate">{tag.name}</span>
            </button>
          );
        })}

        {hasMoreTags && (
          <button
            onClick={() => onShowAllTagsChange(!showAllTags)}
            className="podcast-library-filter-more"
            title={showAllTags ? "收起" : "展开"}
            aria-label={showAllTags ? "收起标签" : "展开标签"}
            aria-expanded={showAllTags}
          >
            {showAllTags ? (
              <IconChevronUp aria-hidden="true" stroke={1.8} />
            ) : (
              <IconChevronDown aria-hidden="true" stroke={1.8} />
            )}
          </button>
        )}

        {selectedTagIds.length > 0 && (
          <button
            onClick={() => onTagToggle(null)}
            className="podcast-library-filter-clear"
          >
            清空
          </button>
        )}
      </div>
    </section>
  );
}
