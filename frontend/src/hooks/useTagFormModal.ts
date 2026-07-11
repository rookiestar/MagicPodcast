import { useCallback, useEffect, useState } from "react";
import {
  DEFAULT_TAG_COLOR,
  getTagFormInitialValues,
  getTagFormPayload,
  getTagFormSubmitError,
  shouldDisableTagFormSubmit,
  shouldSubmitTagFormByKeyboard,
  validateTagFormName,
  type TagFormValues,
} from "@/lib/tagFormState";

interface UseTagFormModalOptions {
  isOpen: boolean;
  initialData?: TagFormValues;
  onClose: () => void;
  onSubmit: (data: TagFormValues) => Promise<void>;
}

export function useTagFormModal({
  isOpen,
  initialData,
  onClose,
  onSubmit,
}: UseTagFormModalOptions) {
  const [name, setName] = useState("");
  const [color, setColor] = useState(DEFAULT_TAG_COLOR);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const resetForm = useCallback(() => {
    setName("");
    setColor(DEFAULT_TAG_COLOR);
    setError("");
  }, []);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const values = getTagFormInitialValues(initialData);
    setName(values.name);
    setColor(values.color);
    setError("");
  }, [initialData, isOpen]);

  const close = useCallback(() => {
    resetForm();
    onClose();
  }, [onClose, resetForm]);

  const submit = useCallback(async () => {
    const validationError = validateTagFormName(name);
    if (validationError) {
      setError(validationError);
      return;
    }

    setLoading(true);
    setError("");

    try {
      await onSubmit(getTagFormPayload(name, color));
      close();
    } catch (err) {
      setError(getTagFormSubmitError(err));
    } finally {
      setLoading(false);
    }
  }, [close, color, name, onSubmit]);

  const handleKeyboardSubmit = useCallback(
    (key: string, metaKey: boolean) => {
      if (shouldSubmitTagFormByKeyboard(key, metaKey)) {
        void submit();
      }
    },
    [submit],
  );

  return {
    name,
    setName,
    color,
    setColor,
    error,
    loading,
    submitDisabled: shouldDisableTagFormSubmit(name, loading),
    close,
    submit,
    handleKeyboardSubmit,
  };
}
