"use client";

interface TagFormFooterProps {
  loading: boolean;
  submitDisabled: boolean;
  buttonText: string;
  onClose: () => void;
  onSubmit: () => void;
}

export default function TagFormFooter({
  loading,
  submitDisabled,
  buttonText,
  onClose,
  onSubmit,
}: TagFormFooterProps) {
  return (
    <div className="editorial-modal-footer p-6">
      <div className="flex justify-end gap-3">
        <button
          onClick={onClose}
          disabled={loading}
          className="editorial-btn editorial-btn--ghost"
        >
          取消
        </button>
        <button
          onClick={onSubmit}
          disabled={submitDisabled}
          className="editorial-btn editorial-btn--primary"
        >
          {buttonText}
        </button>
      </div>
      <p className="mt-2 text-xs text-center" style={{ color: "#6f685e" }}>
        提示：按 Cmd+Enter 快速保存
      </p>
    </div>
  );
}
