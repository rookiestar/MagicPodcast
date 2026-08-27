"use client";

import { useMemo } from "react";
import RichText from "@/components/RichText";
import { stripHtml } from "@/lib/textUtils";

interface EpisodeShowNotesProps {
  html: string;
  link: string;
  isExpanded: boolean;
  onOriginalOpen?: () => void;
}

export function EpisodeShowNotes({
  html,
  link,
  isExpanded,
  onOriginalOpen,
}: EpisodeShowNotesProps) {
  const preview = useMemo(() => stripHtml(html, 220), [html]);

  return (
    <div className="relative">
      <div
        className={`relative max-h-20 overflow-hidden text-xs text-slate-600 transition-[max-height] duration-300 md:text-sm md:text-slate-600 md:dark:text-slate-400 ${
          isExpanded
            ? "md:max-h-96 md:overflow-y-auto"
            : "md:max-h-24 md:overflow-hidden"
        }`}
      >
        {isExpanded ? (
          <RichText
            html={html}
            density="compact"
            className="line-clamp-3 md:line-clamp-none"
          />
        ) : (
          <p className="line-clamp-3 whitespace-pre-line">{preview}</p>
        )}
      </div>

      <div
        className={`absolute bottom-0 left-0 right-0 h-6 bg-gradient-to-t from-white dark:from-slate-800 to-transparent pointer-events-none md:h-8 ${
          isExpanded ? "md:hidden" : ""
        }`}
      />

      {link && (
        <a
          href={link}
          target="_blank"
          rel="noopener noreferrer"
          className="block rounded-sm text-center text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 mt-2 py-2 border-t border-slate-200 dark:border-slate-700 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2 md:hidden"
          onClick={onOriginalOpen}
        >
          查看详情 →
        </a>
      )}
    </div>
  );
}
