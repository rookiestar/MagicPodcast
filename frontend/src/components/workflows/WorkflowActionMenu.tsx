"use client";

import { useState, useEffect, useRef } from "react";

interface WorkflowActionMenuProps {
  workflow: {
    id: number;
    is_enabled: boolean;
    name: string;
  };
  onToggle: (id: number) => void;
  onEdit: (id: number) => void;
  onDelete: (id: number) => void;
}

/**
 * 移动端工作流更多操作菜单
 * 下拉菜单包含：启用/停用、编辑、删除
 */
export default function WorkflowActionMenu({
  workflow,
  onToggle,
  onEdit,
  onDelete,
}: WorkflowActionMenuProps) {
  const [isOpen, setIsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // 点击外部关闭菜单
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => {
        document.removeEventListener("mousedown", handleClickOutside);
      };
    }
  }, [isOpen]);

  const handleToggle = () => {
    onToggle(workflow.id);
    setIsOpen(false);
  };

  const handleEdit = () => {
    onEdit(workflow.id);
    setIsOpen(false);
  };

  const handleDelete = () => {
    onDelete(workflow.id);
    setIsOpen(false);
  };

  return (
    <div className="relative" ref={menuRef}>
      {/* 菜单按钮 */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className="w-10 h-10 flex items-center justify-center rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors active:scale-95"
        aria-label="更多操作"
        aria-expanded={isOpen}
      >
        <svg
          className="w-5 h-5"
          fill="currentColor"
          viewBox="0 0 24 24"
        >
          <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2 2 .9 2 2zm0 2c1.1 0 2-.9 2-2s-.9-2-2-2 2 .9 2 2zm0 6c1.1 0 2-.9 2-2s-.9-2-2-2 2 .9 2 2z" />
        </svg>
      </button>

      {/* 下拉菜单 */}
      {isOpen && (
        <div className="absolute right-0 mt-1 w-40 bg-white rounded-lg shadow-lg border border-slate-200 z-50 overflow-hidden">
          {/* 启用/停用 */}
          <button
            onClick={handleToggle}
            className="w-full text-left px-4 py-3 text-sm text-slate-700 hover:bg-slate-50 transition-colors flex items-center gap-2"
          >
            {workflow.is_enabled ? (
              <>
                <svg
                  className="w-4 h-4 text-amber-600"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z"
                  />
                </svg>
                <span>停用</span>
              </>
            ) : (
              <>
                <svg
                  className="w-4 h-4 text-green-600"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.266a1 1 0 001.555.832l3.197 2.132a1 1 0 001.555-.832V9.87a1 1 0 00-.555.832z"
                  />
                </svg>
                <span>启用</span>
              </>
            )}
          </button>

          {/* 编辑 */}
          <button
            onClick={handleEdit}
            className="w-full text-left px-4 py-3 text-sm text-slate-700 hover:bg-slate-50 transition-colors flex items-center gap-2 border-t border-slate-100"
          >
            <svg
              className="w-4 h-4 text-slate-600"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2V7a2 2 0 00-2-2h-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-5h2v5a2 2 0 002 2zM15 7H9"
              />
            </svg>
            <span>编辑</span>
          </button>

          {/* 删除 */}
          <button
            onClick={handleDelete}
            className="w-full text-left px-4 py-3 text-sm text-red-600 hover:bg-red-50 transition-colors flex items-center gap-2 border-t border-slate-100"
          >
            <svg
              className="w-4 h-4"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 011-1h2a1 1 0 011 1v3M4 7h16"
              />
            </svg>
            <span>删除</span>
          </button>
        </div>
      )}
    </div>
  );
}
