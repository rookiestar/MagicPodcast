interface PodcastNotesEditorProps {
  notes: string;
  isEditingNotes: boolean;
  isSavingNotes?: boolean;
  textareaRows: number;
  editButtonClassName: string;
  saveButtonClassName: string;
  cancelButtonClassName: string;
  readOnlyClassName: string;
  emptyClassName: string;
  onNotesChange: (notes: string) => void;
  onEditNotes: () => void;
  onSaveNotes: () => void;
  onCancelNotesEdit: () => void;
}

export default function PodcastNotesEditor({
  notes,
  isEditingNotes,
  isSavingNotes = false,
  textareaRows,
  editButtonClassName,
  saveButtonClassName,
  cancelButtonClassName,
  readOnlyClassName,
  emptyClassName,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
}: PodcastNotesEditorProps) {
  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <span className="font-semibold text-slate-900">备注：</span>
        {!isEditingNotes && (
          <button
            type="button"
            onClick={onEditNotes}
            className={editButtonClassName}
          >
            编辑
          </button>
        )}
      </div>
      {isEditingNotes ? (
        <div className="space-y-2">
          <textarea
            value={notes}
            onChange={(event) => onNotesChange(event.target.value)}
            disabled={isSavingNotes}
            className="w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 focus:border-transparent focus:ring-2 focus:ring-blue-500"
            rows={textareaRows}
            placeholder="添加备注..."
          />
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onSaveNotes}
              disabled={isSavingNotes}
              className={`${saveButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              {isSavingNotes ? "保存中..." : "保存"}
            </button>
            <button
              type="button"
              onClick={onCancelNotesEdit}
              disabled={isSavingNotes}
              className={`${cancelButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              取消
            </button>
          </div>
        </div>
      ) : (
        <p className={readOnlyClassName}>
          {notes || <span className={emptyClassName}>暂无备注</span>}
        </p>
      )}
    </div>
  );
}
