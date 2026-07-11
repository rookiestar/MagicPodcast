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
    <div className="border-t border-slate-200 dark:border-slate-700 p-6 bg-slate-50 dark:bg-slate-900/50">
      <div className="flex justify-end gap-3">
        <button
          onClick={onClose}
          disabled={loading}
          className="px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors disabled:opacity-50"
        >
          取消
        </button>
        <button
          onClick={onSubmit}
          disabled={submitDisabled}
          className="px-6 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
        >
          {buttonText}
        </button>
      </div>
      <p className="mt-2 text-xs text-slate-500 dark:text-slate-400 text-center">
        提示：按 Cmd+Enter 快速保存
      </p>
    </div>
  );
}
