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
    <div className="editorial-modal-backdrop fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="editorial-modal shadow-2xl w-full max-w-lg overflow-hidden flex flex-col"
        onKeyDown={(event) =>
          handleKeyboardSubmit(event.key, event.metaKey)
        }
      >
        {/* Header */}
        <div className="editorial-modal-header flex items-center justify-between p-6">
          <h2>
            {title}
          </h2>
          <button
            onClick={close}
            disabled={loading}
            className="editorial-modal-close"
            aria-label="关闭"
          >
            ×
          </button>
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
