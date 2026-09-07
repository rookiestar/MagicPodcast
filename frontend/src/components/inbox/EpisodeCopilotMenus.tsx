"use client";

import type { KeyboardEventHandler, RefObject } from "react";
import { IconCheck, IconFileText } from "@tabler/icons-react";
import type {
  EpisodeCopilotContextScope,
  EpisodeCopilotProfile,
  EpisodeCopilotProfileID,
} from "@/types/episodeCopilot";
import styles from "./InboxPage.module.css";

// Product presentation order: speed first, default in the middle, depth last.
const profilePresentation: ReadonlyArray<{
  id: EpisodeCopilotProfileID;
  name: string;
}> = [
  { id: "quick", name: "快速" },
  { id: "balanced", name: "均衡" },
  { id: "deep", name: "深度" },
];

export function profileDisplayName(id: string) {
  return profilePresentation.find((profile) => profile.id === id)?.name ?? id;
}

function profileTechnicalLabel(profile: EpisodeCopilotProfile) {
  const speed =
    profile.service_tier_name || profile.service_tier || "standard";
  return [profile.model, profile.effort, speed].join(" · ");
}

function profileConsumesMoreCredits(profile: EpisodeCopilotProfile) {
  return profile.service_tier_name === "Fast";
}

export function orderedProfiles(
  scope: EpisodeCopilotContextScope,
): EpisodeCopilotProfile[] {
  const byID = new Map(
    (scope.profiles ?? []).map((profile) => [profile.id, profile]),
  );
  return profilePresentation
    .map(({ id }) => byID.get(id))
    .filter((profile): profile is EpisodeCopilotProfile => profile != null);
}

// Task-oriented hints stay in the first tier of the menu; models, efforts,
// speed tiers, and credits move into the collapsible technical section.
const profileTaskHints: Record<EpisodeCopilotProfileID, string> = {
  quick: "快速查找与简短回答",
  balanced: "速度与完整度兼顾",
  deep: "复杂问题与深入分析",
};

interface ProfileMenuProps {
  id: string;
  menuRef: RefObject<HTMLDivElement | null>;
  profiles: EpisodeCopilotProfile[];
  selectedID: EpisodeCopilotProfileID;
  rejectedIDs: ReadonlySet<EpisodeCopilotProfileID>;
  onSelect: (profileID: EpisodeCopilotProfileID) => void;
  onKeyDown: KeyboardEventHandler<HTMLDivElement>;
}

export function EpisodeCopilotProfileMenu({
  id,
  menuRef,
  profiles,
  selectedID,
  rejectedIDs,
  onSelect,
  onKeyDown,
}: ProfileMenuProps) {
  return (
    <div id={id} className={styles.copilotMenu} ref={menuRef}>
      <p className={styles.copilotMenuHeading}>回答方式</p>
      <div role="menu" aria-label="选择回答档位" onKeyDown={onKeyDown}>
        {profiles.map((profile) => {
          const isRejected = rejectedIDs.has(profile.id);
          const isSelected = profile.id === selectedID;
          const name = profileDisplayName(profile.id);
          const hint = profileTaskHints[profile.id];
          return (
            <button
              key={profile.id}
              type="button"
              role="menuitemradio"
              data-menu-item=""
              className={styles.copilotMenuItem}
              aria-checked={isSelected}
              aria-disabled={isRejected || undefined}
              aria-label={`${name}：${hint}${profile.is_default ? "（推荐）" : ""}${isRejected ? "，已确认不可用" : ""}`}
              onClick={() => {
                if (isRejected) return;
                onSelect(profile.id);
              }}
            >
              <span className={styles.copilotMenuItemText}>
                <strong>
                  {name}
                  {profile.is_default && (
                    <span className={styles.copilotMenuBadge}>推荐</span>
                  )}
                  {isRejected && (
                    <span className={styles.copilotMenuUnavailableTag}>
                      已确认不可用
                    </span>
                  )}
                </strong>
                <em>{hint}</em>
              </span>
              {isSelected && (
                <IconCheck size={16} stroke={2} aria-hidden="true" />
              )}
            </button>
          );
        })}
      </div>
      <details className={styles.copilotTechnical}>
        <summary>查看技术信息</summary>
        <p className={styles.copilotTechnicalIntro}>
          问题与当前单集上下文会交给 Mac mini
          上的本地 Codex Runtime；回答只读，不会修改你的内容。
        </p>
        <ul className={styles.copilotTechnicalList}>
          {profiles.map((profile) => (
            <li key={profile.id}>
              <strong>{profileDisplayName(profile.id)}</strong>
              <span>
                {profileTechnicalLabel(profile)}
                {profileConsumesMoreCredits(profile)
                  ? " · 消耗更多 credits"
                  : ""}
              </span>
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}

interface ContextMenuProps {
  id: string;
  menuRef: RefObject<HTMLDivElement | null>;
  scope: EpisodeCopilotContextScope;
  includePrivateNote: boolean;
  isActive: boolean;
  onTogglePrivateNote: (include: boolean) => void;
  onKeyDown: KeyboardEventHandler<HTMLDivElement>;
}

export function EpisodeCopilotContextMenu({
  id,
  menuRef,
  scope,
  includePrivateNote,
  isActive,
  onTogglePrivateNote,
  onKeyDown,
}: ContextMenuProps) {
  return (
    <div
      id={id}
      className={styles.copilotMenu}
      ref={menuRef}
      role="group"
      aria-label="本次回答上下文"
      onKeyDown={onKeyDown}
    >
      <p className={styles.copilotMenuHeading}>本次回答使用</p>
      <ul className={styles.copilotContextList}>
        <li data-available={scope.show_notes_available}>
          <IconFileText size={15} stroke={1.7} aria-hidden="true" />
          <span>Show Notes</span>
          <em>{scope.show_notes_available ? "可用" : "不可用"}</em>
        </li>
        <li data-available={scope.transcript_available}>
          <IconFileText size={15} stroke={1.7} aria-hidden="true" />
          <span>逐字稿</span>
          <em>{scope.transcript_available ? "可用" : "无逐字稿"}</em>
        </li>
      </ul>
      {!scope.transcript_available && scope.show_notes_available && (
        <p className={styles.copilotMenuHint}>
          当前无成功逐字稿，将明确降级为 Show Notes。
        </p>
      )}
      {!scope.transcript_available && !scope.show_notes_available && (
        <p className={styles.copilotMenuHint}>
          当前无可用 Show Notes 或逐字稿；回答将仅参考单集信息与公开资料。
        </p>
      )}
      {scope.private_note_available ? (
        <label className={styles.copilotContextPrivate}>
          <input
            type="checkbox"
            data-menu-item=""
            checked={includePrivateNote}
            disabled={isActive}
            onChange={(event) => onTogglePrivateNote(event.target.checked)}
          />
          <span>
            <strong>本次包含我的私有备注</strong>
            <small>仅进入关闭全部工具的最终回答，不进入公开搜索。</small>
          </span>
        </label>
      ) : (
        <p className={styles.copilotMenuHint}>当前单集没有私有备注。</p>
      )}
    </div>
  );
}
