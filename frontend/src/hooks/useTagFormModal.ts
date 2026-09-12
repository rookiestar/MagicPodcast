import { useCallback, useEffect, useRef, useState } from "react";
import { useUnsavedNavigation } from "@/lib/navigation";
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

  const initialName = initialData?.name;
  const initialColor = initialData?.color;
  const baseline = useRef({ name: "", color: DEFAULT_TAG_COLOR });
  const dirty = isOpen && (name !== baseline.current.name || color !== baseline.current.color);
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const formLocation = useRef("");
  useEffect(() => {
    if (isOpen) {
      const url = new URL(window.location.href);
      formLocation.current = url.pathname + ":" + url.searchParams.get("dialog") + ":" + url.searchParams.get("tag");
    }
  }, [isOpen, initialName, initialColor]);
  useUnsavedNavigation(dirty, (href) => {
    if (!dirtyRef.current) return true;
    const url = new URL(href, "http://navigation.local");
    return url.pathname + ":" + url.searchParams.get("dialog") + ":" + url.searchParams.get("tag") === formLocation.current;
  });

  const resetForm = useCallback(() => {
    baseline.current = { name: "", color: DEFAULT_TAG_COLOR };
    dirtyRef.current = false;
    setName("");
    setColor(DEFAULT_TAG_COLOR);
    setError("");
  }, []);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    const values = getTagFormInitialValues(initialName === undefined ? undefined : { name: initialName, color: initialColor ?? DEFAULT_TAG_COLOR });
    baseline.current = values;
    setName(values.name);
    setColor(values.color);
    setError("");
  }, [initialName, initialColor, isOpen]);

  const close = useCallback(() => {
    if (dirtyRef.current && !window.confirm("有未保存的修改，确定放弃？")) return;
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
      resetForm();
      onClose();
    } catch (err) {
      setError(getTagFormSubmitError(err));
    } finally {
      setLoading(false);
    }
  }, [color, name, onSubmit, onClose, resetForm]);

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
