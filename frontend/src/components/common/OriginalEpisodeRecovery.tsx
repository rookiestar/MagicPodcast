"use client";

import styles from "./OriginalEpisodeRecovery.module.css";

interface OriginalEpisodeRecoveryProps {
  copyError?: string | null;
  onRetry: () => void;
  onOpenApp: () => void;
  onCopy: () => void;
  onDismiss: () => void;
}

export function OriginalEpisodeRecovery({
  copyError,
  onRetry,
  onOpenApp,
  onCopy,
  onDismiss,
}: OriginalEpisodeRecoveryProps) {
  return (
    <div
      className={styles.recovery}
      role="region"
      aria-label="原节目页恢复"
    >
      <p className={styles.message}>
        原节目已尝试打开。如果新页面是 403，可以重试、用小宇宙打开，或复制页面链接。
      </p>
      {copyError ? (
        <p className={styles.copyError} role="alert">
          {copyError}
        </p>
      ) : null}
      <div className={styles.actions}>
        <button type="button" className={styles.action} onClick={onRetry}>
          重试打开
        </button>
        <button type="button" className={styles.action} onClick={onOpenApp}>
          用小宇宙打开
        </button>
        <button type="button" className={styles.action} onClick={onCopy}>
          复制页面链接
        </button>
        <button
          type="button"
          className={styles.dismiss}
          onClick={onDismiss}
          aria-label="关闭原节目页恢复"
        >
          关闭
        </button>
      </div>
    </div>
  );
}
