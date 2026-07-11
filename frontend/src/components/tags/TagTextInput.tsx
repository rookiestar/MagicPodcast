"use client";

import {
  memo,
  type ChangeEventHandler,
  type FocusEventHandler,
  type KeyboardEventHandler,
} from "react";

interface TagTextInputProps {
  value: string;
  placeholder: string;
  disabled: boolean;
  onChangeValue: (value: string) => void;
  onKeyDown: KeyboardEventHandler<HTMLInputElement>;
  onBlur: FocusEventHandler<HTMLInputElement>;
  onFocus: FocusEventHandler<HTMLInputElement>;
}

function TagTextInput({
  value,
  placeholder,
  disabled,
  onChangeValue,
  onKeyDown,
  onBlur,
  onFocus,
}: TagTextInputProps) {
  const handleChange: ChangeEventHandler<HTMLInputElement> = (event) => {
    onChangeValue(event.target.value);
  };

  return (
    <input
      type="text"
      value={value}
      onChange={handleChange}
      onKeyDown={onKeyDown}
      onBlur={onBlur}
      onFocus={onFocus}
      disabled={disabled}
      className={`
        w-full px-4 py-2
        border border-slate-300 dark:border-slate-600
        rounded-lg
        bg-white dark:bg-slate-800
        text-sm text-slate-900 dark:text-slate-100
        placeholder:text-slate-400 dark:placeholder:text-slate-500
        focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent
        disabled:cursor-not-allowed disabled:bg-slate-100 disabled:text-slate-400
        dark:disabled:bg-slate-900 dark:disabled:text-slate-500
        transition-colors
      `}
      placeholder={placeholder}
    />
  );
}

export default memo(TagTextInput);
