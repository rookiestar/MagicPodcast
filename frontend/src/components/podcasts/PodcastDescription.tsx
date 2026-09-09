"use client";

import { useState } from "react";
import RichText from "@/components/RichText";
import {
  getPodcastDescriptionHtml,
  shouldOfferPodcastDescriptionToggle,
} from "@/lib/podcastDetailDisplay";

interface PodcastDescriptionProps {
  description?: string | null;
}

export function PodcastDescription({ description }: PodcastDescriptionProps) {
  const [expanded, setExpanded] = useState(false);
  const canToggle = shouldOfferPodcastDescriptionToggle(description);
  const descriptionHtml = getPodcastDescriptionHtml(description);
  const clamped = canToggle && !expanded;

  return (
    <section className="podcast-reading-description">
      <h2>节目简介</h2>
      <RichText
        html={descriptionHtml}
        className={clamped ? "podcast-reading-description-clamp" : undefined}
      />
      {canToggle && (
        <button
          type="button"
          className="podcast-reading-description-toggle"
          aria-expanded={expanded}
          onClick={() => setExpanded((open) => !open)}
        >
          {expanded ? "收起" : "查看全文"}
        </button>
      )}
    </section>
  );
}
