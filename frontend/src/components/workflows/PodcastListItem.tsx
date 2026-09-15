import React, { memo } from "react";
import { IconPlus, IconX } from "@tabler/icons-react";
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
    const action = `${isSelected ? "移除" : "添加"}节目：${podcast.title}`;
    return (
      <button
        type="button"
        className="workflow-podcast-choice"
        aria-label={action}
        aria-pressed={isSelected}
        data-tooltip={action}
        onClick={() => isSelected ? onRemove(podcast.id) : onAdd(podcast.id)}
      >
        <span className="workflow-podcast-choice-cover">
          <PodcastCover
            coverUrl={getEffectiveCoverUrl(podcast.custom_cover_url, podcast.cover_url)}
            title={podcast.title}
            index={index}
            priority="low"
            sizes="40px"
          />
        </span>
        <span className="workflow-podcast-choice-copy">
          <strong>{podcast.title}</strong>
          {podcast.author && <small>{podcast.author}</small>}
        </span>
        <span className="workflow-podcast-choice-action" aria-hidden="true">
          {isSelected ? <IconX size={18} /> : <IconPlus size={18} />}
        </span>
      </button>
    );
  },
);
PodcastListItem.displayName = "PodcastListItem";
