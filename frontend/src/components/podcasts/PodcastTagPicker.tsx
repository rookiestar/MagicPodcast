"use client";

import { IconX } from "@tabler/icons-react";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { useAvailableTags } from "@/hooks/useAvailableTags";
import { useTagInputActions } from "@/hooks/useTagInputActions";
import {
  getPodcastTagCreateLabel,
  getPodcastTagPanelItems,
  getPodcastTagPanelKeyboardAction,
  togglePodcastDetailTag,
} from "@/lib/podcastTagPickerState";
import { removePodcastDetailTag } from "@/lib/podcastTagControlsState";
import type { Tag } from "@/types";

interface PodcastTagPickerProps {
  tags: Tag[];
  isUpdatingTags?: boolean;
  onTagsChange: (tags: Tag[]) => void;
}

export function PodcastTagPicker({
  tags,
  isUpdatingTags = false,
  onTagsChange,
}: PodcastTagPickerProps) {
  const listId = useId();
  const addButtonRef = useRef<HTMLButtonElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlightedIndex, setHighlightedIndex] = useState(0);
  const {
    availableTags,
    loading,
    ensureAvailableTags,
    appendAvailableTag,
  } = useAvailableTags();
  const { createTag } = useTagInputActions({
    selectedTags: tags,
    onTagsChange,
    disabled: isUpdatingTags,
    appendAvailableTag,
    resetAfterTagChange: () => setQuery(""),
  });
  const items = useMemo(
    () => getPodcastTagPanelItems(availableTags, tags, query),
    [availableTags, query, tags],
  );

  useEffect(() => {
    if (!open) {
      return;
    }

    void ensureAvailableTags();
    setHighlightedIndex(0);
    const frame = window.requestAnimationFrame(() => {
      searchRef.current?.focus();
    });
    return () => window.cancelAnimationFrame(frame);
  }, [ensureAvailableTags, open]);

  useEffect(() => {
    setHighlightedIndex(0);
  }, [query, items.length]);

  const closePanel = (restoreFocus = true) => {
    setOpen(false);
    setQuery("");
    setHighlightedIndex(0);
    if (restoreFocus) {
      addButtonRef.current?.focus();
    }
  };

  const confirmItem = (index: number) => {
    const item = items[index];
    if (!item || isUpdatingTags) {
      return;
    }

    if (item.type === "create") {
      void createTag(item.name);
      return;
    }

    onTagsChange(togglePodcastDetailTag(tags, item.tag));
  };

  const handleSearchKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    const action = getPodcastTagPanelKeyboardAction({
      key: event.key,
      items,
      highlightedIndex,
    });

    if (action.preventDefault) {
      event.preventDefault();
    }

    if (action.type === "highlight") {
      setHighlightedIndex(action.index);
      return;
    }

    if (action.type === "confirm") {
      confirmItem(items.indexOf(action.item));
      return;
    }

    if (action.type === "close") {
      closePanel(true);
    }
  };

  return (
    <div className="podcast-tag-picker">
      <div className="podcast-tag-chip-row">
        {tags.map((tag) => (
          <span key={tag.id} className="podcast-tag-chip" title={tag.name}>
            <span
              className="podcast-tag-chip-dot"
              style={{ backgroundColor: tag.color }}
              aria-hidden="true"
            />
            <span className="podcast-tag-chip-name">{tag.name}</span>
            <button
              type="button"
              className="podcast-tag-chip-remove"
              disabled={isUpdatingTags}
              aria-label={`删除标签 ${tag.name}`}
              onClick={() =>
                onTagsChange(removePodcastDetailTag(tags, tag.id))
              }
            >
              <IconX aria-hidden="true" stroke={1.8} />
            </button>
          </span>
        ))}
        <button
          ref={addButtonRef}
          type="button"
          className="podcast-tag-add"
          disabled={isUpdatingTags}
          aria-expanded={open}
          aria-haspopup="dialog"
          onClick={() => {
            if (open) {
              closePanel(false);
              return;
            }
            setOpen(true);
          }}
        >
          ＋ 添加标签
        </button>
      </div>

      {open && (
        <div className="podcast-tag-panel-shell">
          <button
            type="button"
            className="podcast-tag-panel-backdrop"
            tabIndex={-1}
            aria-label="关闭标签面板"
            onClick={() => closePanel(true)}
          />
          <div
            className="podcast-tag-panel"
            role="dialog"
            aria-label="添加标签"
            onKeyDown={(event) => {
              if (event.key === "Escape" && !event.defaultPrevented) {
                event.preventDefault();
                event.stopPropagation();
                closePanel(true);
              }
            }}
          >
            <input
              ref={searchRef}
              type="search"
              role="combobox"
              className="podcast-tag-panel-search"
              value={query}
              disabled={isUpdatingTags}
              placeholder="搜索标签"
              aria-autocomplete="list"
              aria-haspopup="listbox"
              aria-controls={listId}
              aria-expanded="true"
              onChange={(event) => setQuery(event.target.value)}
              onKeyDown={handleSearchKeyDown}
            />
            <ul id={listId} className="podcast-tag-panel-list" role="listbox">
              {loading && items.length === 0 ? (
                <li className="podcast-tag-panel-empty">正在读取标签…</li>
              ) : (
                items.map((item, index) => {
                  const highlighted = index === highlightedIndex;

                  if (item.type === "create") {
                    return (
                      <li key={`create-${item.name}`} role="presentation">
                        <button
                          type="button"
                          role="option"
                          aria-selected={highlighted}
                          className={`podcast-tag-panel-option${
                            highlighted ? " is-highlighted" : ""
                          }`}
                          disabled={isUpdatingTags}
                          onMouseEnter={() => setHighlightedIndex(index)}
                          onClick={() => confirmItem(index)}
                        >
                          {getPodcastTagCreateLabel(item.name)}
                        </button>
                      </li>
                    );
                  }

                  return (
                    <li key={item.tag.id} role="presentation">
                      <button
                        type="button"
                        role="option"
                        aria-selected={item.selected}
                        className={`podcast-tag-panel-option${
                          highlighted ? " is-highlighted" : ""
                        }`}
                        disabled={isUpdatingTags}
                        onMouseEnter={() => setHighlightedIndex(index)}
                        onClick={() => confirmItem(index)}
                      >
                        <span
                          className={`podcast-tag-panel-check${
                            item.selected ? " is-checked" : ""
                          }`}
                          aria-hidden="true"
                        />
                        <span
                          className="podcast-tag-chip-dot"
                          style={{ backgroundColor: item.tag.color }}
                          aria-hidden="true"
                        />
                        <span className="podcast-tag-chip-name">
                          {item.tag.name}
                        </span>
                      </button>
                    </li>
                  );
                })
              )}
            </ul>
          </div>
        </div>
      )}
    </div>
  );
}
