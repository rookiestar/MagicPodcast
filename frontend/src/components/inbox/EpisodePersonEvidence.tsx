"use client";

import { roleLabel } from "./episodeCopilotMention";

const sourceLabels: Record<string, string> = {
  transcript: "逐字稿", show_notes: "Show Notes", podcast_title: "节目名称",
  podcast_author: "节目作者", podcast_description: "节目简介", episode_title: "单集标题",
};
function record(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : null;
}
export function personEvidence(locator?: string) {
  const rows: { label: string; source: string; quote: string; fragment?: number }[] = [];
  if (!locator) return rows;
  let proof: Record<string, unknown> | null;
  try { proof = record(JSON.parse(locator)); } catch { return rows; }
  if (!proof) return rows;
  const add = (label: string, value: unknown) => {
    const evidence = record(value);
    if (!evidence || typeof evidence.quote !== "string" || !evidence.quote.trim() || typeof evidence.source !== "string" || !sourceLabels[evidence.source]) return;
    rows.push({ label, source: sourceLabels[evidence.source], quote: evidence.quote,
      ...(evidence.source === "transcript" && typeof evidence.fragment === "number" && Number.isInteger(evidence.fragment) && evidence.fragment > 0 ? { fragment: evidence.fragment } : {}),
    });
  };
  add("姓名依据", proof.name_evidence);
  add("本集出场依据", proof.presence_evidence);
  add("角色依据", proof.role_evidence);
  if (Array.isArray(proof.role_conflicts)) for (const item of proof.role_conflicts) {
    const conflict = record(item);
    if (conflict && typeof conflict.role === "string") add(`角色依据有分歧（${roleLabel(conflict.role)}）`, conflict.evidence);
  }
  if (Array.isArray(proof.source_names)) for (const item of proof.source_names) {
    const sourceName = record(item);
    if (sourceName && typeof sourceName.name === "string") add(`转写称呼：${sourceName.name}`, sourceName.evidence);
  }
  if (Array.isArray(proof.speech_bindings)) for (const item of proof.speech_bindings) {
    const binding = record(item);
    if (binding) add("发言归属依据", binding.evidence);
  }
  return rows;
}

export default function EpisodePersonEvidence({ locator, name, onLocate }: {
  locator?: string; name: string; onLocate: (fragment: number) => void;
}) {
  const evidence = personEvidence(locator);
  return <details>
    <summary>查看姓名、角色与原文依据</summary>
    <p>本集显示为{name}。转写中的原始称呼保留不变。</p>
    {evidence.length ? evidence.map((row, index) => <div key={`${row.label}-${index}`}>
      <strong>{row.label}</strong> · {row.source}
      <blockquote>{row.quote}</blockquote>
      {row.fragment ? <button type="button" onClick={() => onLocate(row.fragment!)}>查看原文片段 {row.fragment}</button> : null}
    </div>) : <p>当前记录没有可展示的识别依据，请重新识别或核对原文后纠正。</p>}
  </details>;
}
