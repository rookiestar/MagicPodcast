"use client";

import { useTagFormModal } from "@/hooks/useTagFormModal";
import {
  getTagFormSubmitLabel,
  getTagFormTitle,
} from "@/lib/tagFormState";
import TagFormFields from "./TagFormFields";
import TagFormFooter from "./TagFormFooter";

interface TagFormModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (data: { name: string; color: string }) => Promise<void>;
  initialData?: { name: string; color: string };
  mode: "create" | "edit";
}

export default function TagFormModal({
  isOpen,
  onClose,
  onSubmit,
  initialData,
  mode,
}: TagFormModalProps) {
  const {
    name,
    setName,
    color,
    setColor,
    error,
    loading,
    submitDisabled,
    close,
    submit,
    handleKeyboardSubmit,
  } = useTagFormModal({ isOpen, initialData, onClose, onSubmit });

  if (!isOpen) return null;

  const title = getTagFormTitle(mode);
  const buttonText = getTagFormSubmitLabel(mode, loading);

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div
        className="bg-white dark:bg-slate-800 rounded-lg shadow-2xl w-full max-w-lg overflow-hidden flex flex-col"
        onKeyDown={(event) =>
          handleKeyboardSubmit(event.key, event.metaKey)
        }
      >
        {/* Header */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-slate-50">
              {title}
            </h2>
            <button
              onClick={close}
              disabled={loading}
              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-2xl disabled:opacity-50"
              aria-label="关闭"
            >
              ×
            </button>
          </div>
        </div>

        <TagFormFields
          name={name}
          color={color}
          error={error}
          loading={loading}
          onNameChange={setName}
          onColorChange={setColor}
        />

        <TagFormFooter
          loading={loading}
          submitDisabled={submitDisabled}
          buttonText={buttonText}
          onClose={close}
          onSubmit={submit}
        />
      </div>
    </div>
  );
}
