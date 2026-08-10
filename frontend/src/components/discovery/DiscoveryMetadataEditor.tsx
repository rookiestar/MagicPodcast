"use client";

import { useState } from "react";
import { IconX } from "@tabler/icons-react";
import PodcastNotesEditor from "@/components/podcasts/PodcastNotesEditor";
import { DesktopPodcastTagControls } from "@/components/podcasts/PodcastTagControls";
import {
  type DiscoveryMetadataTarget,
  useDiscoveryMetadataEditing,
} from "@/hooks/useDiscoveryMetadataEditing";

interface DiscoveryMetadataEditorProps {
  episodeId: number;
  podcastId: number;
  onClose: () => void;
}

const targetLabels: Record<DiscoveryMetadataTarget, string> = {
  episode: "Episode",
  podcast: "Podcast",
};

export default function DiscoveryMetadataEditor({
  episodeId,
  podcastId,
  onClose,
}: DiscoveryMetadataEditorProps) {
  const [target, setTarget] =
    useState<DiscoveryMetadataTarget>("episode");
  const metadata = useDiscoveryMetadataEditing({
    target,
    episodeId,
    podcastId,
  });
  const isBusy = metadata.isSavingNotes || metadata.isUpdatingTags;

  return (
    <aside
      id="discovery-metadata-editor"
      className="discovery-metadata-panel"
      aria-label="标签与备注编辑"
    >
      <header className="discovery-metadata-header">
        <div
          className="discovery-metadata-tabs"
          role="tablist"
          aria-label="编辑对象"
        >
          {(["episode", "podcast"] as const).map((item) => (
            <button
              key={item}
              type="button"
              role="tab"
              aria-selected={target === item}
              aria-controls="discovery-metadata-content"
              disabled={isBusy}
              onClick={() => setTarget(item)}
            >
              {targetLabels[item]}
            </button>
          ))}
        </div>
        <button
          type="button"
          className="discovery-metadata-close"
          aria-label="收起编辑"
          title="收起编辑"
          onClick={onClose}
        >
          <IconX aria-hidden="true" stroke={1.8} />
        </button>
      </header>

      <div
        id="discovery-metadata-content"
        className="discovery-metadata-content"
        role="tabpanel"
        aria-label={`${targetLabels[target]} 标签与备注`}
      >
        {metadata.isLoading ? (
          <p className="discovery-metadata-state">正在加载标签与备注…</p>
        ) : metadata.isError ? (
          <div className="discovery-metadata-state is-error" role="alert">
            <p>标签与备注加载失败。</p>
            <button type="button" onClick={metadata.reload}>
              重试
            </button>
          </div>
        ) : metadata.isLoaded ? (
          <>
            <DesktopPodcastTagControls
              tags={metadata.tags}
              isUpdatingTags={isBusy}
              onTagsChange={(tags) => void metadata.handleTagsChange(tags)}
            />
            <PodcastNotesEditor
              notes={metadata.notes}
              isEditingNotes={metadata.isEditingNotes}
              isSavingNotes={isBusy}
              textareaRows={5}
              editButtonClassName="podcast-management-link"
              saveButtonClassName="podcast-management-primary"
              cancelButtonClassName="podcast-management-secondary"
              readOnlyClassName="podcast-notes-readonly"
              emptyClassName="podcast-notes-empty"
              onNotesChange={metadata.setNotes}
              onEditNotes={() => metadata.setIsEditingNotes(true)}
              onSaveNotes={() => void metadata.handleNotesSave()}
              onCancelNotesEdit={metadata.cancelNotesEdit}
            />
          </>
        ) : null}
      </div>
    </aside>
  );
}
