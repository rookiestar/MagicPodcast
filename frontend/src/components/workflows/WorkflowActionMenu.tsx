"use client";

import { useState, useEffect, useRef } from "react";

import { IconDots, IconPlayerPause, IconPlayerPlay, IconEdit, IconTrash } from "@tabler/icons-react";

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
 * 工作流共用的更多操作菜单
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
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [isOpen]);

  const handleToggle = () => { setIsOpen(false); onToggle(workflow.id); };
  const handleEdit = () => { setIsOpen(false); onEdit(workflow.id); };
  const handleDelete = () => { setIsOpen(false); onDelete(workflow.id); };

  return (

    <div className="workflow-more" ref={menuRef} onKeyDown={(event) => {
      if (event.key === "Escape") { setIsOpen(false); menuRef.current?.querySelector<HTMLButtonElement>("button")?.focus(); }
    }} onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget)) setIsOpen(false);
    }}>
      <button type="button" onClick={() => setIsOpen(!isOpen)} className="workflow-quiet-action" aria-label={`更多操作：${workflow.name}`} aria-expanded={isOpen}>
        <IconDots size={20} aria-hidden="true" />
      </button>
      {isOpen && (
        <div className="workflow-more-panel" aria-label={`${workflow.name}的操作`}>
          <button type="button" onClick={handleEdit}><IconEdit size={16} aria-hidden="true" />编辑</button>
          <button type="button" onClick={handleToggle}>
            {workflow.is_enabled ? <IconPlayerPause size={16} aria-hidden="true" /> : <IconPlayerPlay size={16} aria-hidden="true" />}
            {workflow.is_enabled ? "停用" : "启用"}
          </button>
          <button type="button" className="is-danger" onClick={handleDelete}><IconTrash size={16} aria-hidden="true" />删除</button>
        </div>
      )}
    </div>
  );
}
