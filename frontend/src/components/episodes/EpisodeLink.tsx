"use client";

import Link from "next/link";
import type { ComponentProps } from "react";
import { rememberEpisodeOrigin } from "@/lib/navigation";

type Props = ComponentProps<typeof Link> & { episodeID: number; source: string };

/** Keep a normal link (including new-tab gestures) and record same-tab provenance. */
export default function EpisodeLink({ episodeID, source, onClick, id, ...props }: Props) {
  const triggerID = id ?? `episode-entry-${source}-${episodeID}`;
  return <Link {...props} id={triggerID} onClick={(event) => {
    onClick?.(event);
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || props.target === "_blank") return;
    rememberEpisodeOrigin(episodeID, triggerID);
  }} />;
}
