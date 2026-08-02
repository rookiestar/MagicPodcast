"use client";

import {
  IconChevronDown,
  IconChevronUp,
  IconExternalLink,
  IconFlame,
  IconPlayerPlay,
} from "@tabler/icons-react";
import { useState } from "react";
import RichText from "@/components/RichText";
import {
  formatPodcastLatestEpisodeDurationLabel,
  formatPodcastNewestEpisodeDate,
  getPodcastDescriptionHtml,
  getPodcastDetailInfoCoverUrl,
  shouldShowPodcastLatestEpisodePlayButton,
  shouldShowPodcastPopularityBadge,
  shouldShowPodcastWebsiteLink,
} from "@/lib/podcastDetailDisplay";
import type { Podcast, Tag } from "@/types";
import PodcastCover from "./PodcastCover";
import PodcastNotesEditor from "./PodcastNotesEditor";
import {
  DesktopPodcastTagControls,
  MobilePodcastTagControls,
} from "./PodcastTagControls";

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
  const [detailsOpen, setDetailsOpen] = useState(false);
  const coverUrl = getPodcastDetailInfoCoverUrl(podcast);
  const showLatestEpisodePlayButton =
    shouldShowPodcastLatestEpisodePlayButton(podcast.newest_enclosure_url);
  const showWebsiteLink = shouldShowPodcastWebsiteLink(podcast.link);
  const showPopularityBadge = shouldShowPodcastPopularityBadge(
    podcast.popularity_score,
  );
  const descriptionHtml = getPodcastDescriptionHtml(podcast.description);

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
          <p className="podcast-reading-kicker">个人播客库 · 节目档案</p>
          <h1>{podcast.title}</h1>
          <p className="podcast-reading-mobile-meta">
            {podcast.author} · {podcast.episode_count || 0} 集
          </p>
        </div>
      </header>

      <div className="podcast-reading-mobile-actions">
        {showLatestEpisodePlayButton && (
          <button
            type="button"
            onClick={() =>
              window.open(podcast.newest_enclosure_url, "_blank")
            }
            className="podcast-reading-primary-action"
          >
            <IconPlayerPlay aria-hidden="true" stroke={1.8} />
            播放最新一集
          </button>
        )}
        <button
          type="button"
          aria-expanded={detailsOpen}
          onClick={() => setDetailsOpen((open) => !open)}
          className="podcast-reading-secondary-action"
        >
          {detailsOpen ? (
            <IconChevronUp aria-hidden="true" stroke={1.8} />
          ) : (
            <IconChevronDown aria-hidden="true" stroke={1.8} />
          )}
          {detailsOpen ? "收起详细信息" : "展开详细信息"}
        </button>
      </div>

      <details open={detailsOpen}>
        <summary className="hidden" />
        <div
          className="podcast-reading-mobile-management"
          role="region"
          aria-label="节目管理"
        >
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
            <div className="podcast-reading-popularity">
              <IconFlame aria-hidden="true" stroke={1.7} />
              热度 {podcast.popularity_score}/10
            </div>
          )}

          <section className="podcast-reading-description">
            <h2>节目简介</h2>
            <div className="line-clamp-3">
              <RichText html={descriptionHtml} />
            </div>
          </section>

          <MobilePodcastTagControls
            tags={tags}
            isUpdatingTags={isUpdatingTags}
            onTagsChange={onTagsChange}
          />

          <PodcastNotesEditor
            notes={notes}
            isEditingNotes={isEditingNotes}
            isSavingNotes={isSavingNotes}
            textareaRows={3}
            editButtonClassName="podcast-management-link"
            saveButtonClassName="podcast-management-primary"
            cancelButtonClassName="podcast-management-secondary"
            readOnlyClassName="podcast-notes-readonly line-clamp-2"
            emptyClassName="podcast-notes-empty"
            onNotesChange={onNotesChange}
            onEditNotes={onEditNotes}
            onSaveNotes={onSaveNotes}
            onCancelNotesEdit={onCancelNotesEdit}
          />
        </div>
      </details>
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
  const durationLabel = formatPodcastLatestEpisodeDurationLabel(
    podcast.newest_enclosure_duration,
  );
  const showLatestEpisodePlayButton =
    shouldShowPodcastLatestEpisodePlayButton(podcast.newest_enclosure_url);
  const showWebsiteLink = shouldShowPodcastWebsiteLink(podcast.link);
  const showPopularityBadge = shouldShowPodcastPopularityBadge(
    podcast.popularity_score,
  );
  const descriptionHtml = getPodcastDescriptionHtml(podcast.description);

  return (
    <article
      className="podcast-reading-hero hidden md:grid"
      aria-label={podcast.title}
    >
      <figure className="podcast-reading-cover">
        <PodcastCover
          coverUrl={coverUrl}
          title={podcast.title}
          priority="low"
          sizes="(max-width: 1200px) 240px, 300px"
        />
      </figure>

      <section className="podcast-reading-copy">
        <p className="podcast-reading-kicker">个人播客库 · 节目档案</p>
        <h1>{podcast.title}</h1>

        <dl className="podcast-reading-metadata">
          <div>
            <dt>主播</dt>
            <dd>{podcast.author}</dd>
          </div>
          <div>
            <dt>单集</dt>
            <dd>{podcast.episode_count || 0}</dd>
          </div>
          <div>
            <dt>最近更新</dt>
            <dd>{formatPodcastNewestEpisodeDate(podcast.newest_episode_date)}</dd>
          </div>
        </dl>

        <section className="podcast-reading-description">
          <h2>节目简介</h2>
          <RichText html={descriptionHtml} />
        </section>

        <div className="podcast-reading-links">
          {showLatestEpisodePlayButton && (
            <button
              type="button"
              onClick={() =>
                window.open(podcast.newest_enclosure_url, "_blank")
              }
              className="podcast-reading-primary-action"
              title="播放最新一集"
            >
              <IconPlayerPlay aria-hidden="true" stroke={1.8} />
              播放最新一集
              {durationLabel && <small>{durationLabel}</small>}
            </button>
          )}
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
      </section>

      <aside
        className="podcast-reading-management"
        role="region"
        aria-label="节目管理"
      >
        <div className="podcast-reading-management-heading">
          <span>个人管理</span>
          <small>标签与备注</small>
        </div>
        <DesktopPodcastTagControls
          tags={tags}
          isUpdatingTags={isUpdatingTags}
          onTagsChange={onTagsChange}
        />
        <PodcastNotesEditor
          notes={notes}
          isEditingNotes={isEditingNotes}
          isSavingNotes={isSavingNotes}
          textareaRows={4}
          editButtonClassName="podcast-management-link"
          saveButtonClassName="podcast-management-primary"
          cancelButtonClassName="podcast-management-secondary"
          readOnlyClassName="podcast-notes-readonly"
          emptyClassName="podcast-notes-empty"
          onNotesChange={onNotesChange}
          onEditNotes={onEditNotes}
          onSaveNotes={onSaveNotes}
          onCancelNotesEdit={onCancelNotesEdit}
        />
      </aside>
    </article>
  );
}
