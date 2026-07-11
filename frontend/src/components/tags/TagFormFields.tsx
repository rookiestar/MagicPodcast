"use client";

import ColorPicker from "@/components/ui/ColorPicker";

interface TagFormFieldsProps {
  name: string;
  color: string;
  error: string;
  loading: boolean;
  onNameChange: (name: string) => void;
  onColorChange: (color: string) => void;
}

export default function TagFormFields({
  name,
  color,
  error,
  loading,
  onNameChange,
  onColorChange,
}: TagFormFieldsProps) {
  return (
    <div className="p-6 space-y-6">
      {error && (
        <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-800 dark:text-red-400 px-4 py-3 rounded-lg">
          {error}
        </div>
      )}

      <div>
        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
          标签名称 <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          placeholder="输入标签名称"
          disabled={loading}
          className="w-full px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white dark:bg-slate-700 text-slate-900 dark:text-slate-100 disabled:opacity-50"
          maxLength={50}
          autoFocus
        />
        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
          {name.length}/50
        </p>
      </div>

      <div>
        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
          标签颜色
        </label>
        <ColorPicker
          value={color}
          onChange={onColorChange}
          disabled={loading}
        />
      </div>
    </div>
  );
}
