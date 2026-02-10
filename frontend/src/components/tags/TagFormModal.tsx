"use client";

import { useState, useEffect } from "react";
import ColorPicker from "@/components/ui/ColorPicker";

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
  const [name, setName] = useState("");
  const [color, setColor] = useState("#3B82F6");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  // 初始化表单数据
  useEffect(() => {
    if (isOpen) {
      if (initialData) {
        setName(initialData.name);
        setColor(initialData.color);
      } else {
        setName("");
        setColor("#3B82F6");
      }
      setError("");
    }
  }, [isOpen, initialData]);

  // 重置表单
  const handleClose = () => {
    setName("");
    setColor("#3B82F6");
    setError("");
    onClose();
  };

  // 提交表单
  const handleSubmit = async () => {
    // 验证
    if (!name.trim()) {
      setError("标签名称不能为空");
      return;
    }

    setLoading(true);
    setError("");

    try {
      await onSubmit({ name: name.trim(), color });
      // 成功后关闭
      handleClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "操作失败，请重试");
    } finally {
      setLoading(false);
    }
  };

  // 键盘快捷键
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && e.metaKey) {
      // Cmd+Enter 提交
      handleSubmit();
    }
  };

  if (!isOpen) return null;

  const title = mode === "create" ? "新建" : "编辑";
  const buttonText = loading ? "保存中..." : mode === "create" ? "创建" : "保存";

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4">
      <div
        className="bg-white dark:bg-slate-800 rounded-lg shadow-2xl w-full max-w-lg overflow-hidden flex flex-col"
        onKeyDown={handleKeyDown}
      >
        {/* Header */}
        <div className="border-b border-slate-200 dark:border-slate-700 p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-2xl font-bold text-slate-900 dark:text-slate-50">
              {title}
            </h2>
            <button
              onClick={handleClose}
              disabled={loading}
              className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 text-2xl disabled:opacity-50"
              aria-label="关闭"
            >
              ×
            </button>
          </div>
        </div>

        {/* Content */}
        <div className="p-6 space-y-6">
          {/* 错误提示 */}
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-800 dark:text-red-400 px-4 py-3 rounded-lg">
              {error}
            </div>
          )}

          {/* 标签名称 */}
          <div>
            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              标签名称 <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
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

          {/* 标签颜色 */}
          <div>
            <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
              标签颜色
            </label>
            <ColorPicker
              value={color}
              onChange={setColor}
              disabled={loading}
            />
          </div>
        </div>

        {/* Footer */}
        <div className="border-t border-slate-200 dark:border-slate-700 p-6 bg-slate-50 dark:bg-slate-900/50">
          <div className="flex justify-end gap-3">
            <button
              onClick={handleClose}
              disabled={loading}
              className="px-4 py-2 border border-slate-300 dark:border-slate-600 rounded-lg text-slate-700 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors disabled:opacity-50"
            >
              取消
            </button>
            <button
              onClick={handleSubmit}
              disabled={loading || !name.trim()}
              className="px-6 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed font-medium"
            >
              {buttonText}
            </button>
          </div>
          <p className="mt-2 text-xs text-slate-500 dark:text-slate-400 text-center">
            提示：按 Cmd+Enter 快速保存
          </p>
        </div>
      </div>
    </div>
  );
}
