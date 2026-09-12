"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { createPortal } from "react-dom";
import { IconPencil, IconUsers, IconX } from "@tabler/icons-react";
import { episodeCopilotApi } from "@/lib/api/episodeCopilot";
import type {
  EpisodePeoplePayload,
  PersonReviewDraft,
  PersonReviewMatch,
} from "@/types/episodeCopilot";
import type { TranscriptSegment } from "@/types/processing";
import EpisodePersonEvidence from "./EpisodePersonEvidence";
import styles from "./TranscriptPeople.module.css";

function trapTab(event: KeyboardEvent<HTMLElement>) {
  if (event.key !== "Tab") return;
  const controls = Array.from(
    event.currentTarget.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex="0"]',
    ),
  ).filter((el) => el.getClientRects().length > 0);
  const first = controls[0],
    last = controls.at(-1);
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
}

export function useTranscriptPeople(
  episodeId: number | undefined,
  artifactSetId: number,
  segments: TranscriptSegment[],
  locate: (order: number) => void,
) {
  const [people, setPeople] = useState<EpisodePeoplePayload | null>(null);
  const [draft, setDraft] = useState<PersonReviewDraft | null>(null);
  const [history, setHistory] = useState<PersonReviewDraft[]>([]);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [dirty, setDirty] = useState(false);
  const [saved, setSaved] = useState("");
  const [editor, setEditor] = useState<{
    order: number;
    name: string;
    personId: number;
    scope: string;
  } | null>(null);
  const pending = draft?.matches.some((m) => !m.applied) ?? false;
  const panelElement = useRef<HTMLElement>(null);
  const editorElement = useRef<HTMLElement>(null);
  const request = useRef<AbortController | null>(null);
  const generation = useRef(0);
  useEffect(() => {
    if (!open) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    panelElement.current?.querySelector<HTMLButtonElement>("button")?.focus();
    return () => previous?.focus();
  }, [open]);
  const editing = editor !== null;
  useEffect(() => {
    if (!editing) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    editorElement.current
      ?.querySelector<HTMLInputElement>('input[type="text"],input:not([type])')
      ?.focus();
    return () => previous?.focus();
  }, [editing]);
  const sourceVersion = `artifact-${artifactSetId}`;
  const accept = useCallback((value: EpisodePeoplePayload) => {
    setPeople(value);
    setDraft(value.draft ?? null);
    setDirty(false);
  }, []);
  useEffect(() => {
    if (!episodeId) return;
    const controller = new AbortController();
    const requestGeneration = generation;
    setHistory([]);
    setPeople(null);
    setDraft(null);
    setOpen(false);
    setEditor(null);
    setDirty(false);
    setError("");
    episodeCopilotApi
      .getPeople(episodeId, controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) accept(value);
      })
      .catch(() => {
        if (!controller.signal.aborted)
          setError("人物资料读取失败，可重试；逐字稿仍可阅读。");
      });
    return () => {
      controller.abort();
      request.current?.abort();
      requestGeneration.current++;
    };
  }, [episodeId, artifactSetId, accept]);
  const current = people?.source_version === sourceVersion;
  const applied = current
    ? (people?.attributions ?? []).filter(
        (a) => a.status === "confirmed" && a.user_confirmed && a.person_id,
      )
    : [];
  const nameFor = (segment: TranscriptSegment) =>
    applied.find((a) => a.fragment_order === segment.order)?.display_name ||
    segment.speaker;
  const notify = () =>
    window.dispatchEvent(
      new CustomEvent("episode-people-changed", { detail: episodeId }),
    );
  const run = async (
    label: string,
    action: () => Promise<EpisodePeoplePayload>,
    appliedChange = false,
  ) => {
    const version = ++generation.current;
    setBusy(label);
    setError("");
    try {
      const value = await action();
      if (generation.current !== version) return false;
      accept(value);
      setSaved(appliedChange ? "已应用" : "已保存");
      if (appliedChange) notify();
      return true;
    } catch (e) {
      if (generation.current === version)
        setError(
          e instanceof Error ? e.message : "保存失败，修改尚未应用，请重试。",
        );
      return false;
    } finally {
      if (generation.current === version) setBusy("");
    }
  };
  const loadHistory = async () => {
    if (!episodeId) return;
    try {
      const version = generation.current;
      const result = await episodeCopilotApi.peopleDrafts(episodeId);
      if (generation.current === version) setHistory(result);
    } catch {
      setError("草稿读取失败，请重试。");
    }
  };
  const prepare = async () => {
    if (!episodeId || dirty) return;
    request.current = new AbortController();
    setOpen(true);
    await run("正在识别人物…", () =>
      episodeCopilotApi.preparePeople(episodeId, request.current!.signal),
    );
    await loadHistory();
  };
  const review = async (apply: boolean) => {
    if (!episodeId || !draft) return;
    const selected = draft.matches.filter((m) => m.selected);
    if (apply && selected.length === 0) return;
    const success = await run(
      apply ? "正在应用…" : "正在保存…",
      () =>
        episodeCopilotApi.reviewPeople(
          episodeId,
          {
            draft_id: draft.id,
            revision: people?.revision ?? 0,
            source_version: draft.source_version,
            matches: draft.matches,
          },
          apply,
        ),
      apply,
    );
    if (success) await loadHistory();
  };
  const editMatch = (key: string, patch: Partial<PersonReviewMatch>) => {
    setDraft((d) =>
      d
        ? {
            ...d,
            matches: d.matches.map((m) =>
              m.key === key ? { ...m, ...patch, applied: false } : m,
            ),
          }
        : d,
    );
    setDirty(true);
    setSaved("");
  };
  const editSpeaker = (segment: TranscriptSegment) => {
    if (dirty) {
      setError("草稿尚未保存，请先保存或放弃本次修改。");
      return;
    }
    const assigned = applied.find((a) => a.fragment_order === segment.order);
    const ids = new Set(
      applied
        .filter((a) => a.speaker_label === segment.speaker)
        .map((a) => a.person_id),
    );
    const group = segments.filter((s) => s.speaker === segment.speaker);
    const mixed =
      ids.size > 1 ||
      (ids.size === 1 &&
        applied.filter((a) => a.speaker_label === segment.speaker).length !==
          group.length);
    setEditor({
      order: segment.order,
      name: assigned?.display_name ?? "",
      personId: assigned?.person_id ?? 0,
      scope: mixed ? "fragment" : "speaker",
    });
    setError("");
  };
  const saveManual = async (clear = false) => {
    if (!episodeId || !editor) return;
    if (dirty) {
      setError("草稿尚未保存，请先保存或放弃本次修改。");
      return;
    }
    const success = await run(
      clear ? "正在解除匹配…" : "正在应用…",
      () =>
        episodeCopilotApi.manualPerson(episodeId, {
          revision: people?.revision ?? 0,
          source_version: sourceVersion,
          fragment_order: editor.order,
          scope: editor.scope,
          person_id: editor.personId,
          display_name: editor.name.trim(),
          clear,
        }),
      true,
    );
    if (success) setEditor(null);
  };
  const anchor = editor && segments.find((s) => s.order === editor.order);
  const count = anchor
    ? editor?.scope === "speaker"
      ? segments.filter((s) => s.speaker === anchor.speaker).length
      : 1
    : 0;
  const names = [
    ...new Set(applied.map((a) => a.display_name).filter(Boolean)),
  ];
  const close = () => {
    if (dirty) {
      setError("草稿尚未保存，请先保存，或放弃本次修改。");
      return;
    }
    setOpen(false);
  };
  const reload = () =>
    episodeId && run("正在读取…", () => episodeCopilotApi.getPeople(episodeId));
  return {
    open,
    nameFor,
    speakerLabel: (segment: TranscriptSegment) =>
      episodeId ? (
        <span className={styles.label}>
          <button
            type="button"
            className={styles.name}
            onDoubleClick={() => editSpeaker(segment)}
            onClick={(e) => e.stopPropagation()}
            title="双击编辑姓名"
          >
            {nameFor(segment)}
          </button>
          <button
            type="button"
            className={styles.pencil}
            aria-label={`编辑${nameFor(segment)}，片段 ${segment.order}`}
            onClick={() => editSpeaker(segment)}
          >
            <IconPencil size={14} />
          </button>
        </span>
      ) : (
        <span>{segment.speaker}</span>
      ),
    toolbar: episodeId ? (
      <div className={styles.toolbar}>
        <span className={styles.heading}>
          <IconUsers size={17} />
          人物与发言
        </span>
        <span className={styles.summary}>
          {names.length
            ? `${names.slice(0, 3).join(" · ")}${names.length > 3 ? ` 等 ${names.length} 人` : ""}`
            : "尚未确认人物"}
        </span>
        {draft && (
          <span className={styles.status}>
            {draft.outdated ? "需重新核对" : pending ? "待确认" : "已确认"}
          </span>
        )}
        <button
          type="button"
          disabled={!!busy}
          onClick={() => {
            setOpen(true);
            void loadHistory();
          }}
        >
          {pending ? "继续核对" : names.length || draft ? "管理" : "识别人物"}
        </button>
        {error && !open && !editor && (
          <span role="alert">
            {error}{" "}
            <button type="button" onClick={() => void reload()}>
              重试
            </button>
          </span>
        )}
      </div>
    ) : null,
    panel:
      open && episodeId ? (
        <aside
          ref={panelElement}
          className={styles.panel}
          aria-label="人物与发言核对"
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.stopPropagation();
              close();
            }
            if (window.innerWidth <= 1000) {
              e.stopPropagation();
              trapTab(e);
            }
          }}
        >
          <header>
            <div>
              <h3>人物与发言</h3>
              <p>核对姓名与具体发言，确认后才会生效。</p>
            </div>
            <button type="button" aria-label="关闭人物核对" onClick={close}>
              <IconX size={18} />
            </button>
          </header>
          <div className={styles.panelBody}>
            {error && (
              <p role="alert" className={styles.error}>
                {error}
                <button type="button" onClick={() => void reload()}>
                  重新读取
                </button>
              </p>
            )}
            <div className={styles.actions}>
              <button
                type="button"
                disabled={!!busy || dirty}
                onClick={() => void prepare()}
              >
                {draft ? "重新识别" : "开始识别"}
              </button>
              {busy === "正在识别人物…" && (
                <button
                  type="button"
                  onClick={() => {
                    request.current?.abort();
                    generation.current++;
                    setBusy("");
                    setSaved("已取消，已保存结果保留");
                  }}
                >
                  取消识别
                </button>
              )}
            </div>
            <p role="status">{busy || (dirty ? "修改尚未保存" : saved)}</p>
            {history.length > 1 && (
              <label>
                识别记录
                <select
                  aria-label="选择识别记录"
                  value={draft?.id ?? ""}
                  disabled={dirty || !!busy}
                  onChange={(e) =>
                    setDraft(
                      history.find((d) => d.id === Number(e.target.value)) ??
                        null,
                    )
                  }
                >
                  {history.map((d) => (
                    <option key={d.id} value={d.id}>
                      记录 {d.id}
                      {d.outdated ? " · 旧来源" : ""}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {draft?.outdated && (
              <p className={styles.error}>
                来源已变化，此记录仅供核对。请重新识别当前逐字稿。
              </p>
            )}
            {draft?.matches.map((match) => {
              const group = segments.filter(
                (s) => s.speaker === match.speaker_label,
              );
              const before = [
                ...new Set(
                  match.orders.map(
                    (order) =>
                      applied.find((a) => a.fragment_order === order)
                        ?.display_name ||
                      segments.find((s) => s.order === order)?.speaker ||
                      "未匹配",
                  ),
                ),
              ];
              const adjusted =
                match.applied &&
                match.orders.some(
                  (order) =>
                    applied.find((a) => a.fragment_order === order)
                      ?.person_id !== match.person_id,
                );
              return (
                <section className={styles.match} key={match.key}>
                  <label className={styles.matchTitle}>
                    <input
                      type="checkbox"
                      checked={match.selected}
                      disabled={
                        !!busy || draft.outdated || !match.orders.length
                      }
                      onChange={(e) =>
                        editMatch(match.key, { selected: e.target.checked })
                      }
                    />
                    {match.speaker_label || "尚未匹配 Speaker"}
                    <span>→</span>
                    {match.display_name}
                  </label>
                  <p>
                    {match.applied
                      ? adjusted
                        ? "曾应用，当前归属已有调整"
                        : "已应用"
                      : match.uncertain
                        ? "存在不确定项，请核对"
                        : "识别建议，尚需确认"}{" "}
                    · {match.orders.length} 段
                    {group.length === match.orders.length && group.length > 0
                      ? "（该 Speaker 全部发言）"
                      : "（局部或未匹配）"}
                  </p>
                  {before.length > 0 && (
                    <p>
                      当前：{before.join("、")} → 建议：{match.display_name}
                    </p>
                  )}
                  <div className={styles.fields}>
                    <label>
                      姓名
                      <input
                        aria-label={`姓名 ${match.key}`}
                        value={match.display_name}
                        disabled={!!busy || draft.outdated}
                        onChange={(e) =>
                          editMatch(match.key, { display_name: e.target.value })
                        }
                      />
                    </label>
                    <label>
                      角色
                      <select
                        value={match.role}
                        disabled={!!busy || draft.outdated}
                        onChange={(e) =>
                          editMatch(match.key, {
                            role: e.target.value,
                            role_edited: true,
                          })
                        }
                      >
                        <option value="unknown">未确认</option>
                        <option value="host">主持人</option>
                        <option value="guest">嘉宾</option>
                      </select>
                    </label>
                  </div>
                  <EpisodePersonEvidence
                    name={match.display_name}
                    locator={match.evidence_locator}
                    onLocate={locate}
                  />
                  <details>
                    <summary>核对匹配范围 · {match.orders.length} 段</summary>
                    {group.length === 0 && (
                      <p>尚无可靠发言绑定，可在逐字稿手动编辑 Speaker。</p>
                    )}
                    {group.map((seg) => (
                      <div className={styles.fragment} key={seg.order}>
                        <label>
                          <input
                            type="checkbox"
                            checked={match.orders.includes(seg.order)}
                            disabled={!!busy || draft.outdated}
                            onChange={(e) =>
                              editMatch(match.key, {
                                orders: e.target.checked
                                  ? [...match.orders, seg.order].sort(
                                      (a, b) => a - b,
                                    )
                                  : match.orders.filter((n) => n !== seg.order),
                              })
                            }
                          />
                          {seg.text}
                        </label>
                        <button type="button" onClick={() => locate(seg.order)}>
                          定位 / 试听
                        </button>
                      </div>
                    ))}
                  </details>
                </section>
              );
            })}
            {draft && draft.matches.length === 0 && (
              <p>没有识别出可核对的人物，可直接编辑逐字稿中的 Speaker。</p>
            )}
            {!!people?.excluded_people?.length && (
              <details>
                <summary>已排除人物（{people.excluded_people.length}）</summary>
                {people.excluded_people.map((person) => (
                  <p key={person.id}>
                    {person.display_name}{" "}
                    <button
                      type="button"
                      disabled={!!busy || dirty}
                      onClick={() =>
                        void run(
                          "正在恢复参与…",
                          () =>
                            episodeCopilotApi.correctAppearance(
                              episodeId,
                              person.id,
                              { excluded: false },
                            ),
                          true,
                        )
                      }
                    >
                      恢复参与
                    </button>
                  </p>
                ))}
              </details>
            )}
            {names.length > 0 && (
              <p>
                已应用人物：{names.join("、")}。双击逐字稿姓名可修改或解除匹配。
              </p>
            )}
          </div>
          {draft && (
            <footer>
              <span>{dirty ? "未保存" : "草稿已保存"}</span>
              <button
                type="button"
                disabled={!dirty || !!busy || draft.outdated}
                onClick={() => void review(false)}
              >
                保存草稿
              </button>
              <button
                type="button"
                className={styles.primary}
                disabled={
                  !!busy ||
                  draft.outdated ||
                  !draft.matches.some((m) => m.selected)
                }
                onClick={() => void review(true)}
              >
                确认并应用
              </button>
              {dirty && (
                <button
                  type="button"
                  onClick={() => {
                    setDraft(people?.draft ?? null);
                    setDirty(false);
                    setError("");
                  }}
                >
                  放弃本次修改
                </button>
              )}
            </footer>
          )}
        </aside>
      ) : null,
    editor:
      editor && anchor
        ? createPortal(
            <div
              className={styles.editorBackdrop}
              onKeyDown={(e) => {
                e.stopPropagation();
                if (e.key === "Escape" && !busy) {
                  e.preventDefault();
                  setEditor(null);
                }
                trapTab(e);
              }}
            >
              <section
                ref={editorElement}
                className={styles.editor}
                role="dialog"
                aria-modal="true"
                aria-label="编辑发言人物"
              >
                <h3>编辑发言人物</h3>
                <p>
                  {anchor.speaker} · 片段 {anchor.order}
                </p>
                <label>
                  选择本集人物
                  <select
                    value={editor.personId}
                    disabled={!!busy}
                    onChange={(e) => {
                      const id = Number(e.target.value);
                      const person = people?.people.find((p) => p.id === id);
                      setEditor({
                        ...editor,
                        personId: id,
                        name: person?.display_name ?? "",
                      });
                    }}
                  >
                    <option value={0}>输入新姓名或称呼</option>
                    {people?.people.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.display_name} · 人物 {p.id}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  姓名或称呼
                  <input
                    autoFocus
                    aria-label="姓名或称呼"
                    value={editor.name}
                    maxLength={200}
                    disabled={!!busy}
                    onChange={(e) =>
                      setEditor({ ...editor, name: e.target.value })
                    }
                  />
                </label>
                <fieldset>
                  <legend>应用范围</legend>
                  {[
                    ["speaker", "该 Speaker 的全部发言"],
                    ["fragment", "仅此段"],
                  ].map(([value, label]) => (
                    <label key={value}>
                      <input
                        type="radio"
                        name="speaker-scope"
                        checked={editor.scope === value}
                        disabled={!!busy}
                        onChange={() => setEditor({ ...editor, scope: value })}
                      />
                      {label}
                    </label>
                  ))}
                </fieldset>
                <p>整组修改会覆盖范围内已有归属；仅影响本集。</p>
                {error && (
                  <p role="alert" className={styles.error}>
                    {error}
                  </p>
                )}
                <div className={styles.actions}>
                  <button
                    type="button"
                    disabled={!!busy}
                    onClick={() => setEditor(null)}
                  >
                    取消
                  </button>
                  <button
                    type="button"
                    disabled={
                      !!busy ||
                      !applied.some((a) => a.fragment_order === editor.order)
                    }
                    onClick={() => void saveManual(true)}
                  >
                    解除匹配
                  </button>
                  <button
                    type="button"
                    className={styles.primary}
                    disabled={!!busy || !editor.name.trim() || !people}
                    onClick={() => void saveManual()}
                  >
                    确认应用到 {count} 段
                  </button>
                </div>
              </section>
            </div>,
            document.body,
          )
        : null,
  };
}
