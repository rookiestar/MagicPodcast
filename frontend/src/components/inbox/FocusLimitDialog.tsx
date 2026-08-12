"use client";

import { useEffect, useRef } from "react";
import { IconAlertTriangle, IconTargetArrow } from "@tabler/icons-react";
import type { ConsumptionItem } from "@/types/consumption";
import styles from "./InboxPage.module.css";

interface FocusLimitDialogProps {
  item: ConsumptionItem;
  currentCount: number;
  limit: number;
  isSaving: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

export default function FocusLimitDialog({
  item,
  currentCount,
  limit,
  isSaving,
  onCancel,
  onConfirm,
}: FocusLimitDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  return (
    <div
      className={styles.confirmBackdrop}
      onMouseDown={(event) => {
        if (event.currentTarget === event.target && !isSaving) onCancel();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape" && !isSaving) onCancel();
      }}
    >
      <div
        className={styles.confirmDialog}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="focus-limit-title"
        aria-describedby="focus-limit-description"
      >
        <span className={styles.confirmIcon}>
          <IconAlertTriangle size={24} stroke={1.8} aria-hidden="true" />
        </span>
        <span className={styles.detailKicker}>FOCUS CAPACITY</span>
        <h2 id="focus-limit-title">Focus 已有 {currentCount} 项</h2>
        <p id="focus-limit-description">
          建议上限为 {limit} 项。加入《{item.episode_title}
          》前，你可以先把低优先内容移至 Someday；若仍要承诺，也可明确继续。
        </p>
        <div className={styles.confirmActions}>
          <button
            ref={cancelRef}
            type="button"
            className={styles.confirmSecondary}
            disabled={isSaving}
            onClick={onCancel}
          >
            保持原队列
          </button>
          <button
            type="button"
            className={styles.confirmPrimary}
            disabled={isSaving}
            onClick={onConfirm}
          >
            <IconTargetArrow size={18} stroke={1.8} aria-hidden="true" />
            {isSaving ? "正在加入…" : "仍加入 Focus"}
          </button>
        </div>
      </div>
    </div>
  );
}
