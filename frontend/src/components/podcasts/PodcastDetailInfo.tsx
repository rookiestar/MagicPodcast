"use client";

import { IconExternalLink, IconFlame, IconPlayerPlay } from "@tabler/icons-react";
import {
  formatPodcastDetailMetaLine,
  formatPodcastLatestEpisodeDurationLabel,
  getPodcastDetailInfoCoverUrl,
  shouldShowPodcastLatestEpisodePlayButton,
  shouldShowPodcastPopularityBadge,
  shouldShowPodcastWebsiteLink,
} from "@/lib/podcastDetailDisplay";
import type { Podcast, Tag } from "@/types";
import { PodcastDescription } from "./PodcastDescription";
import PodcastCover from "./PodcastCover";
import PodcastNotesEditor from "./PodcastNotesEditor";
import { PodcastTagPicker } from "./PodcastTagPicker";

interface PodcastDetailInfoProps {
  podcast: Podcast;
  tags: Tag[];
  notes: string;
  isEditingNotes: boolean;
  isSavingNotes?: boolean;
  isUpdatingTags?: boolean;
  onNotesChange: (notes: string) => void;
  onEditNotes: () => void;
  onSaveNotes: () => void;
  onCancelNotesEdit: () => void;
  onTagsChange: (tags: Tag[]) => void;
}

function PodcastDetailPlayback({ podcast }: { podcast: Podcast }) {
  const durationLabel = formatPodcastLatestEpisodeDurationLabel(
    podcast.newest_enclosure_duration,
  );
  const showLatestEpisodePlayButton = shouldShowPodcastLatestEpisodePlayButton(
    podcast.newest_enclosure_url,
  );

  if (!showLatestEpisodePlayButton) {
    return null;
  }

  return (
    <div className="podcast-reading-playback">
      {showLatestEpisodePlayButton && (
        <button
          type="button"
          onClick={() => window.open(podcast.newest_enclosure_url, "_blank")}
          className="podcast-reading-primary-action"
        >
          <IconPlayerPlay aria-hidden="true" stroke={1.8} />
          播放最新一集
        </button>
      )}
      {showLatestEpisodePlayButton && durationLabel && (
        <span className="podcast-reading-duration">{durationLabel}</span>
      )}
    </div>
  );
}

function PodcastDetailSourceLinks({ podcast }: { podcast: Podcast }) {
  const showWebsiteLink = shouldShowPodcastWebsiteLink(podcast.link);
  const showPopularityBadge = shouldShowPodcastPopularityBadge(
    podcast.popularity_score,
  );

  if (!showWebsiteLink && !showPopularityBadge) {
    return null;
  }

  return (
    <div className="podcast-reading-links">
      {showWebsiteLink && (
        <a
          href={podcast.link}
          target="_blank"
          rel="noopener noreferrer"
          className="podcast-reading-source-link"
        >
          节目官网
          <IconExternalLink aria-hidden="true" stroke={1.7} />
        </a>
      )}
      {showPopularityBadge && (
        <span className="podcast-reading-popularity">
          <IconFlame aria-hidden="true" stroke={1.7} />
          热度 {podcast.popularity_score}/10
        </span>
      )}
    </div>
  );
}

function PodcastDetailManagement({
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  textareaRows,
  readOnlyClassName,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
}: Omit<PodcastDetailInfoProps, "podcast"> & {
  textareaRows: number;
  readOnlyClassName: string;
}) {
  return (
    <>
      <div className="podcast-tag-controls text-sm">
        <span className="podcast-management-label">标签</span>
        <div className="mt-2">
          <PodcastTagPicker
            tags={tags}
            isUpdatingTags={isUpdatingTags}
            onTagsChange={onTagsChange}
          />
        </div>
      </div>
      <PodcastNotesEditor
        notes={notes}
        isEditingNotes={isEditingNotes}
        isSavingNotes={isSavingNotes}
        textareaRows={textareaRows}
        editButtonClassName="podcast-management-link"
        saveButtonClassName="podcast-management-primary"
        cancelButtonClassName="podcast-management-secondary"
        readOnlyClassName={readOnlyClassName}
        emptyClassName="podcast-notes-empty"
        onNotesChange={onNotesChange}
        onEditNotes={onEditNotes}
        onSaveNotes={onSaveNotes}
        onCancelNotesEdit={onCancelNotesEdit}
      />
    </>
  );
}

export function MobilePodcastDetailInfo({
  podcast,
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
}: PodcastDetailInfoProps) {
  const coverUrl = getPodcastDetailInfoCoverUrl(podcast);

  return (
    <article
      className="podcast-reading-mobile md:hidden"
      aria-label={podcast.title}
    >
      <header className="podcast-reading-mobile-header">
        <div className="podcast-reading-mobile-cover">
          <PodcastCover
            coverUrl={coverUrl}
            title={podcast.title}
            priority="low"
            sizes="96px"
          />
        </div>
        <div className="min-w-0 flex-1">
          <h1>{podcast.title}</h1>
          <p className="podcast-reading-meta">
            {formatPodcastDetailMetaLine(
              podcast.author,
              podcast.episode_count,
              podcast.newest_episode_date,
            )}
          </p>
        </div>
      </header>

      <PodcastDetailPlayback podcast={podcast} />
      <PodcastDetailSourceLinks podcast={podcast} />
      <PodcastDescription description={podcast.description} />

      <div
        className="podcast-reading-mobile-management"
        role="region"
        aria-label="节目管理"
      >
        <PodcastDetailManagement
          tags={tags}
          notes={notes}
          isEditingNotes={isEditingNotes}
          isSavingNotes={isSavingNotes}
          isUpdatingTags={isUpdatingTags}
          textareaRows={3}
          readOnlyClassName="podcast-notes-readonly"
          onNotesChange={onNotesChange}
          onEditNotes={onEditNotes}
          onSaveNotes={onSaveNotes}
          onCancelNotesEdit={onCancelNotesEdit}
          onTagsChange={onTagsChange}
        />
      </div>
    </article>
  );
}

export function DesktopPodcastDetailInfo({
  podcast,
  tags,
  notes,
  isEditingNotes,
  isSavingNotes,
  isUpdatingTags,
  onNotesChange,
  onEditNotes,
  onSaveNotes,
  onCancelNotesEdit,
  onTagsChange,
}: PodcastDetailInfoProps) {
  const coverUrl = getPodcastDetailInfoCoverUrl(podcast);

  return (
    <article
      className="podcast-reading-hero hidden md:grid"
      aria-label={podcast.title}
    >
      <section className="podcast-reading-copy">
        <header className="podcast-reading-heading">
          <figure className="podcast-reading-cover">
            <PodcastCover
              coverUrl={coverUrl}
              title={podcast.title}
              priority="low"
              sizes="96px"
            />
          </figure>
          <div className="podcast-reading-heading-copy">
            <h1>{podcast.title}</h1>
            <p className="podcast-reading-meta">
              {formatPodcastDetailMetaLine(
                podcast.author,
                podcast.episode_count,
                podcast.newest_episode_date,
              )}
            </p>
            <PodcastDetailPlayback podcast={podcast} />
          </div>
        </header>

        <PodcastDetailSourceLinks podcast={podcast} />
        <PodcastDescription description={podcast.description} />
      </section>

      <aside
        className="podcast-reading-management"
        role="region"
        aria-label="标签与备注"
      >
        <div className="podcast-reading-management-heading">
          <h2>标签与备注</h2>
        </div>
        <PodcastDetailManagement
          tags={tags}
          notes={notes}
          isEditingNotes={isEditingNotes}
          isSavingNotes={isSavingNotes}
          isUpdatingTags={isUpdatingTags}
          textareaRows={4}
          readOnlyClassName="podcast-notes-readonly"
          onNotesChange={onNotesChange}
          onEditNotes={onEditNotes}
          onSaveNotes={onSaveNotes}
          onCancelNotesEdit={onCancelNotesEdit}
          onTagsChange={onTagsChange}
        />
      </aside>
    </article>
  );
}
