"use client";

import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { IconChevronDown } from "@tabler/icons-react";
import FocusLimitDialog from "@/components/inbox/FocusLimitDialog";
import { useMenuPopover } from "@/components/inbox/useMenuPopover";
import { QUEUE_PRESENTATION } from "@/components/inbox/presentation";
import styles from "@/components/inbox/InboxPage.module.css";
import {
  consumptionApi,
  getConsumptionErrorDetails,
  requiresFocusConfirmation,
} from "@/lib/api/consumption";
import type { Episode } from "@/types";
import type { ConsumptionQueue } from "@/types/consumption";

type ActionQueue = Exclude<ConsumptionQueue, "done">;
const TARGETS: ActionQueue[] = ["inbox", "focus", "someday"];

export default function EpisodeQueueMenu({
  episode,
  onQueueChange,
  activeFocusEpisodeId,
  onFocusPromptChange,
}: {
  episode: Episode;
  activeFocusEpisodeId?: number | null;
  onFocusPromptChange?: (episodeId: number, open: boolean) => void;
  onQueueChange?: (episodeId: number, queue: Episode["queue_state"]) => void;
}) {
  const menu = useMenuPopover();
  const [queue, setQueue] = useState(episode.queue_state ?? null);
  const [saving, setSaving] = useState(false);
  const [openAbove, setOpenAbove] = useState(false);
  const [alignLeft, setAlignLeft] = useState(false);
  const mounted = useRef(true);
  const busy = useRef(false);
  const [failure, setFailure] = useState<{
    target: ActionQueue;
    message: string;
  } | null>(null);
  const [focusPrompt, setFocusPrompt] = useState<{
    currentCount: number;
    limit: number;
  } | null>(null);
  const [announcement, setAnnouncement] = useState("");

  useEffect(() => {
    setQueue(episode.queue_state ?? null);
  }, [episode.queue_state]);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      onFocusPromptChange?.(episode.id, false);
    };
  }, [episode.id, onFocusPromptChange]);

  function dismissFocusPrompt() {
    setFocusPrompt(null);
    onFocusPromptChange?.(episode.id, false);
  }

  const waitingForFocus = focusPrompt !== null;
  const label = queue ? QUEUE_PRESENTATION[queue].label : "加入队列";
  const menuLabel = queue === "done" ? "重新处理，加入" : queue ? "切换至" : "加入队列";

  async function move(target: ActionQueue, acknowledgeFocusLimit = false) {
    if (busy.current || target === queue) return;
    busy.current = true;
    setSaving(true);
    setFailure(null);
    setAnnouncement("");
    menu.closeMenu();
    dismissFocusPrompt();
    try {
      const item = await consumptionApi.setQueue(episode.id, target, {
        acknowledgeFocusLimit,
      });
      if (!mounted.current) return;
      setQueue(item.queue_state);
      onQueueChange?.(episode.id, item.queue_state);
      setAnnouncement(`已加入 ${QUEUE_PRESENTATION[target].label}`);
    } catch (error) {
      if (!mounted.current) return;
      const details = getConsumptionErrorDetails(error);
      if (target === "focus" && requiresFocusConfirmation(error)) {
        setFocusPrompt({
          currentCount: details.currentCount ?? 7,
          limit: details.focusLimit ?? 7,
        });
        onFocusPromptChange?.(episode.id, true);
      } else {
        // Retrying a failed Focus write must ask again if the server still
        // requires confirmation; a previous acknowledgement is not retained.
        setFailure({ target, message: details.message });
      }
    } finally {
      busy.current = false;
      setSaving(false);
    }
  }

  return (
    <div className={styles.queueControls}>
      <div className={styles.queueMenu}>
        <button
          ref={menu.triggerRef}
          type="button"
          className={styles.queueMenuTrigger}
          aria-label={`${label}，${episode.title}，打开队列菜单`}
          aria-haspopup="menu"
          aria-expanded={menu.open}
          aria-controls={menu.open ? menu.menuId : undefined}
          aria-disabled={saving || waitingForFocus}
          onClick={() => {
            if (busy.current || waitingForFocus) return;
            const bounds = menu.triggerRef.current?.getBoundingClientRect();
            // Leave room for the mobile bottom navigation as well as the menu.
            setOpenAbove(Boolean(bounds && window.innerHeight - bounds.bottom < 260 && bounds.top > 200));
            setAlignLeft(Boolean(bounds && bounds.right < 196));
            menu.toggleMenu();
          }}
        >
          {label}{saving ? " · 保存中…" : waitingForFocus ? " · 待确认…" : <IconChevronDown size={15} aria-hidden="true" />}
        </button>
        {menu.open && (
          <div
            ref={menu.menuRef}
            id={menu.menuId}
            role="menu"
            aria-label={menuLabel}
            className={styles.queueMenuPopup}
            style={{
              ...(openAbove ? { top: "auto", bottom: "calc(100% + 7px)" } : {}),
              ...(alignLeft ? { left: 0, right: "auto" } : { left: "auto", right: 0 }),
            }}
            onKeyDown={menu.handleMenuKeyDown}
          >
            <span className={styles.queueMenuTitle} aria-hidden="true">{menuLabel}</span>
            {TARGETS.filter((target) => target !== queue).map((target) => (
              <button key={target} type="button" role="menuitem" onClick={() => void move(target)}>
                {QUEUE_PRESENTATION[target].label}
              </button>
            ))}
          </div>
        )}
      </div>
      <span className="sr-only" aria-live="polite" role={saving || waitingForFocus || announcement ? "status" : undefined}>{saving ? "正在保存队列" : waitingForFocus ? "等待 Focus 确认" : announcement}</span>
      {failure && (
        <div role="alert" className="podcast-episode-queue-error">
          <span>{failure.message}</span>
          <button type="button" onClick={() => void move(failure.target)}>重试</button>
        </div>
      )}
      {focusPrompt &&
        (activeFocusEpisodeId === undefined || activeFocusEpisodeId === episode.id) &&
        createPortal(
        <div className={styles.queueControls}>
          <FocusLimitDialog
            item={{ episode_title: episode.title }}
            currentCount={focusPrompt.currentCount}
            limit={focusPrompt.limit}
            isSaving={saving}
            onCancel={() => { dismissFocusPrompt(); menu.closeMenu(); }}
            onConfirm={() => void move("focus", true)}
          />
        </div>,
        document.body,
      )}
    </div>
  );
}
