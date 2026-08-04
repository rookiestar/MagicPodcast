"use client";

import {
  IconChevronDown,
  IconSortDescending2,
} from "@tabler/icons-react";
import { useId, useState } from "react";
import SortDrawer from "@/components/podcasts/SortDrawer";

export interface EditorialSortOption<T extends string = string> {
  label: string;
  value: T;
}

interface EditorialSortControlsProps<T extends string> {
  sortBy: T;
  options: EditorialSortOption<T>[];
  onSortChange: (sortBy: T) => void;
}

/**
 * Shared sort control for editorial list toolbars.
 * Desktop keeps the current value compact; mobile uses the established drawer.
 */
export default function EditorialSortControls<T extends string>({
  sortBy,
  options,
  onSortChange,
}: EditorialSortControlsProps<T>) {
  const [isSortDrawerOpen, setIsSortDrawerOpen] = useState(false);
  const desktopSelectId = useId();

  return (
    <>
      <button
        onClick={() => setIsSortDrawerOpen(true)}
        className="podcast-sort-mobile md:hidden"
        aria-label="排序"
        aria-haspopup="dialog"
        aria-expanded={isSortDrawerOpen}
        type="button"
      >
        <IconSortDescending2 aria-hidden="true" stroke={1.8} />
      </button>

      <div className="podcast-sort-desktop hidden md:flex">
        <IconSortDescending2
          className="podcast-sort-desktop-icon"
          aria-hidden="true"
          stroke={1.7}
        />
        <IconChevronDown
          className="podcast-sort-desktop-chevron"
          aria-hidden="true"
          stroke={1.8}
        />
        <label className="sr-only" htmlFor={desktopSelectId}>
          排序方式
        </label>
        <select
          id={desktopSelectId}
          value={sortBy}
          onChange={(event) => onSortChange(event.target.value as T)}
          className="podcast-sort-select"
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
        onSortChange={(value) => onSortChange(value as T)}
        options={options}
      />
    </>
  );
}
