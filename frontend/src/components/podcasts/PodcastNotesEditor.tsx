import {
  arePodcastNotesControlsDisabled,
  getPodcastNotesReadOnlyText,
  getPodcastNotesSaveButtonLabel,
  hasPodcastNotes,
  shouldShowPodcastNotesEditButton,
} from "@/lib/podcastNotesEditorState";

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
  const showEditButton = shouldShowPodcastNotesEditButton(isEditingNotes);
  const controlsDisabled = arePodcastNotesControlsDisabled(isSavingNotes);
  const saveButtonLabel = getPodcastNotesSaveButtonLabel(isSavingNotes);
  const readOnlyText = getPodcastNotesReadOnlyText(notes);
  const hasNotes = hasPodcastNotes(notes);

  return (
    <div>
      <div className="mb-2 flex items-center justify-between">
        <span className="podcast-management-label">备注</span>
        {showEditButton && (
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
            disabled={controlsDisabled}
            className="podcast-notes-textarea"
            rows={textareaRows}
            placeholder="添加备注..."
          />
          <div className="flex gap-2">
            <button
              type="button"
              onClick={onSaveNotes}
              disabled={controlsDisabled}
              className={`${saveButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              {saveButtonLabel}
            </button>
            <button
              type="button"
              onClick={onCancelNotesEdit}
              disabled={controlsDisabled}
              className={`${cancelButtonClassName} disabled:cursor-not-allowed disabled:opacity-60`}
            >
              取消
            </button>
          </div>
        </div>
      ) : (
        <p className={readOnlyClassName}>
          {hasNotes ? (
            readOnlyText
          ) : (
            <span className={emptyClassName}>{readOnlyText}</span>
          )}
        </p>
      )}
    </div>
  );
}
