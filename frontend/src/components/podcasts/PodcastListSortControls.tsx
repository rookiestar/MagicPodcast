"use client";

import { useState } from "react";
import type {
  PodcastSortBy,
  PodcastSortOption,
} from "@/lib/podcastListState";
import SortDrawer from "./SortDrawer";

interface PodcastListSortControlsProps {
  sortBy: PodcastSortBy;
  options: PodcastSortOption[];
  onSortChange: (sortBy: PodcastSortBy) => void;
}

export default function PodcastListSortControls({
  sortBy,
  options,
  onSortChange,
}: PodcastListSortControlsProps) {
  const [isSortDrawerOpen, setIsSortDrawerOpen] = useState(false);

  return (
    <>
      <button
        onClick={() => setIsSortDrawerOpen(true)}
        className="md:hidden w-10 h-10 flex items-center justify-center rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 active:bg-slate-300 active:scale-95 transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
        aria-label="排序"
        aria-haspopup="dialog"
        aria-expanded={isSortDrawerOpen}
      >
        <svg
          className="w-5 h-5"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M3 4h13M3 8h9m-9 4h6m4 0l4-4m0 0l4 4m-4-4h12"
          />
        </svg>
      </button>

      <div className="hidden md:flex items-center gap-2">
        <label htmlFor="podcast-sort" className="text-sm text-slate-600">
          排序：
        </label>
        <select
          id="podcast-sort"
          value={sortBy}
          onChange={(event) =>
            onSortChange(event.target.value as PodcastSortBy)
          }
          className="
            px-3 py-2 pr-8
            border border-slate-300 rounded-lg
            bg-white text-sm text-slate-700
            focus:ring-2 focus:ring-blue-500 focus:border-transparent
            transition-colors
            appearance-none
            cursor-pointer
          "
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </div>

      <SortDrawer
        isOpen={isSortDrawerOpen}
        onClose={() => setIsSortDrawerOpen(false)}
        currentSort={sortBy}
        onSortChange={(value) => onSortChange(value as PodcastSortBy)}
        options={options}
      />
    </>
  );
}
