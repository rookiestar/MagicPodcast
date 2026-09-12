"use client";

import { useState } from "react";
import type { PersonReviewMatch, SpeakerRelationCandidate } from "@/types/episodeCopilot";
import type { TranscriptSegment } from "@/types/processing";
import styles from "./TranscriptPeople.module.css";

const states: Record<string, string> = {
  direct: "直接证据", inferred: "综合建议 · 待确认", conflict: "存在冲突 · 请选择",
  insufficient: "姓名待确认", not_assessed: "本次未给出判断", invalid_evidence: "证据待核对",
};

export default function SpeakerRelationReview({ match, candidates, group, disabled, adjusted, currentName, onChange, onLocate }: {
  match: PersonReviewMatch;
  candidates: SpeakerRelationCandidate[];
  group: TranscriptSegment[];
  disabled: boolean;
  adjusted: boolean;
  currentName: (segment: TranscriptSegment) => string;
  onChange: (patch: Partial<PersonReviewMatch>) => void;
  onLocate: (order: number) => void;
}) {
  const [editing, setEditing] = useState(false);
  const relation = match.relation!;
  const evidenceCandidates = relation.candidates;
  const hasChoice = !!match.choice && !!match.display_name.trim();
  const changed = group.some(segment => match.orders.includes(segment.order) && currentName(segment) !== segment.speaker && currentName(segment) !== match.display_name);
  return <section className={styles.match} aria-label={`核对 ${match.speaker_label}`}>
    <div className={styles.matchHeader}>
      <label className={styles.matchTitle}>
        <input type="checkbox" aria-label={`应用 ${match.speaker_label}`} checked={match.selected}
          disabled={disabled || !hasChoice || !match.orders.length}
          onChange={event => onChange({ selected: event.target.checked })} />
        {match.speaker_label}<span>→</span>{hasChoice ? match.display_name : "未匹配"}
      </label>
      <button type="button" className={styles.editAction} disabled={disabled} aria-expanded={editing}
        onClick={() => setEditing(!editing)}>{editing ? "收起编辑" : hasChoice ? "更换人物" : "选择人物"}</button>
    </div>
    <p>{match.orders.length} 段{match.orders.length === group.length ? " · 全部发言" : " · 人工调整范围"} · {match.applied ? (adjusted ? "当前归属已调整" : "已应用") : hasChoice && relation.state === "conflict" ? "候选已选择 · 待确认" : states[relation.state] || "待确认"}</p>
    {editing && <div className={styles.fields}>
      <label>人物
        <select aria-label={`${match.speaker_label} 人物`} value={match.choice || ""} disabled={disabled}
          onChange={event => {
            const choice = event.target.value;
            const candidate = candidates.find(person => person.id === choice);
            onChange({ choice, display_name: candidate?.display_name || (choice === "manual" ? "" : match.speaker_label),
              role: candidate?.role || "unknown", selected: false, role_edited: false, person_id: undefined });
          }}>
          <option value="">暂不匹配</option>
          {candidates.map(person => <option key={person.id} value={person.id}>{person.display_name}{candidates.some(other => other.id !== person.id && other.display_name === person.display_name) ? `（${person.identity_note || person.id}）` : ""}</option>)}
          <option value="manual">填写其他姓名</option>
        </select>
      </label>
      {match.choice && <>
        <label>姓名<input aria-label={`${match.speaker_label} 姓名`} value={match.display_name} disabled={disabled}
          onChange={event => onChange({ display_name: event.target.value })} /></label>
        <label>角色<select aria-label={`${match.speaker_label} 角色`} value={match.role} disabled={disabled}
          onChange={event => onChange({ role: event.target.value, role_edited: true })}>
          <option value="unknown">未确认</option><option value="host">主持人</option><option value="guest">嘉宾</option>
        </select></label>
      </>}
    </div>}
    <details>
      <summary>查看依据</summary>
      {relation.reason && <p>{relation.reason}</p>}
      {evidenceCandidates.map(candidate => <div key={candidate.id}>
        <strong>{candidate.display_name}</strong>
        <p>{candidate.reason}</p>
        {[...(candidate.evidence || []).map(e => ({...e, counter: false})), ...(candidate.counter_evidence || []).map(e => ({...e, counter: true}))].map((evidence, index) => <div className={styles.evidencePreview} key={`${candidate.id}:${index}`}>
          <small>{evidence.counter ? "待解决的矛盾" : "支持依据"} · {evidence.source === "transcript" ? `片段 ${evidence.fragment}` : "节目资料"}</small>
          <blockquote>{evidence.quote}</blockquote>
          {evidence.source === "transcript" && evidence.fragment > 0 && <button type="button" onClick={() => onLocate(evidence.fragment)}>查看原文片段 {evidence.fragment}</button>}
        </div>)}
      </div>)}
    </details>
    {changed && <p className={styles.exception}>应用将覆盖所选片段的现有姓名；可展开范围保留人工例外。</p>}
    <details>
      <summary>调整范围 · {match.orders.length} / {group.length} 段</summary>
      {group.map(segment => <div className={styles.fragment} key={segment.order}>
        <label><input type="checkbox" aria-label={`选择片段 ${segment.order}`} disabled={disabled} checked={match.orders.includes(segment.order)}
          onChange={event => onChange({ orders: event.target.checked ? [...match.orders, segment.order].sort((a,b) => a-b) : match.orders.filter(order => order !== segment.order) })} />{segment.text}</label>
        <small>当前：{currentName(segment)} → {match.orders.includes(segment.order) && hasChoice ? match.display_name : "保持原状"}</small>
        <button type="button" onClick={() => onLocate(segment.order)}>定位 / 试听</button>
      </div>)}
    </details>
  </section>;
}
