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
    <div className="space-y-6">
      {error && (
        <div className="editorial-state is-error" style={{ margin: 0, padding: "12px 14px", textAlign: "left" }}>
          <p style={{ margin: 0 }}>{error}</p>
        </div>
      )}

      <div>
        <label className="editorial-label mb-2">
          标签名称 <span style={{ color: "#9d2d20" }}>*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          placeholder="输入标签名称"
          disabled={loading}
          className="editorial-field"
          maxLength={50}
          autoFocus
        />
        <p className="mt-1 text-xs" style={{ color: "#6f685e" }}>
          {name.length}/50
        </p>
      </div>

      <div>
        <label className="editorial-label mb-2">
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
