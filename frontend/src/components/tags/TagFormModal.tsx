"use client";

import { IconX } from "@tabler/icons-react";
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
        role="dialog"
        aria-modal="true"
        aria-labelledby="tag-form-modal-title"
        onKeyDown={(event) =>
          handleKeyboardSubmit(event.key, event.metaKey)
        }
      >
        {/* Header */}
        <div className="editorial-modal-header">
          <div className="editorial-modal-heading">
            <span className="editorial-modal-kicker">标签管理</span>
            <small>{mode === "create" ? "新建" : "编辑"}</small>
          </div>
          <button
            onClick={close}
            disabled={loading}
            className="editorial-modal-close"
            aria-label="关闭"
          >
            <IconX aria-hidden="true" stroke={1.8} />
          </button>
        </div>

        <h2 id="tag-form-modal-title" className="editorial-modal-title">
          {title}
        </h2>

        <div className="editorial-modal-body">
          <TagFormFields
            name={name}
            color={color}
            error={error}
            loading={loading}
            onNameChange={setName}
            onColorChange={setColor}
          />
        </div>

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
