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
import { episodeCopilotApi, type PersonPreparationEvent } from "@/lib/api/episodeCopilot";
import type {
  EpisodePeoplePayload,
  PersonReviewDraft,
  PersonReviewMatch,
} from "@/types/episodeCopilot";
import type { TranscriptSegment } from "@/types/processing";
import EpisodePersonEvidence, { personEvidence } from "./EpisodePersonEvidence";
import styles from "./TranscriptPeople.module.css";

const preparationStages = [
  ["read", "读取逐字稿与节目资料"], ["identify", "识别出场人物"],
  ["review", "核对发言归属"], ["save", "保存待确认草稿"],
] as const;

function PreparationStatus({ stage, started }: { stage: string; started: number }) {
  const [elapsed, setElapsed] = useState(0);
  useEffect(() => {
    const tick = () => setElapsed(Math.floor((performance.now() - started) / 1000));
    tick();
    const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [started]);
  const index = preparationStages.findIndex(([key]) => key === stage);
  return <section className={styles.progress} aria-label="人物识别进度">
    <div className={styles.progressHeading}><span className={styles.spinner} aria-hidden="true" />
      <div><strong role="status">{index < 0 ? "正在连接识别服务" : `正在${preparationStages[index][1]}`}</strong>
        <p aria-live="off">已用时 {String(Math.floor(elapsed / 60)).padStart(2, "0")}:{String(elapsed % 60).padStart(2, "0")}</p></div></div>
    <ol>{preparationStages.map(([key, title], i) => <li key={key} data-state={i < index ? "done" : i === index ? "active" : "waiting"}>
      <span className={styles.stepDot} aria-hidden="true">{i < index ? "✓" : ""}</span>
      <div>{title}<small>{i < index ? "已完成" : i === index ? "正在处理" : "等待进行"}</small></div>
    </li>)}</ol>
  </section>;
}

function trapTab(event: KeyboardEvent<HTMLElement>) {
  if (event.key !== "Tab") return;
  const controls = Array.from(
    event.currentTarget.querySelectorAll<HTMLElement>(
      'button:not(:disabled), input:not(:disabled), select:not(:disabled), summary, a[href], [tabindex="0"]',
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
  const [editingMatch, setEditingMatch] = useState<string | null>(null);
  const [busy, setBusy] = useState("");
  const [progress, setProgress] = useState<{ stage: string; started: number } | null>(null);
  const [needsReadback, setNeedsReadback] = useState(false);
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
  const panelBodyElement = useRef<HTMLDivElement>(null);
  const panelScroll = useRef(0);
  const request = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const operation = useRef(false);
  useEffect(() => {
    if (!open) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    panelElement.current?.querySelector<HTMLButtonElement>("button")?.focus();
    if (panelBodyElement.current) panelBodyElement.current.scrollTop = panelScroll.current;
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
    const panel = panelElement.current;
    return () => queueMicrotask(() => {
      if (previous?.isConnected) previous.focus();
      else panel?.querySelector<HTMLButtonElement>("button")?.focus();
    });
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
    const initialGeneration = generation.current;
    operation.current = false;
    setBusy("");
    setProgress(null);
    setNeedsReadback(false);
    setSaved("");
    setEditingMatch(null);
    panelScroll.current = 0;
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
        if (!controller.signal.aborted && generation.current === initialGeneration) accept(value);
      })
      .catch(() => {
        if (!controller.signal.aborted && generation.current === initialGeneration)
          setError("人物资料读取失败，可重试；逐字稿仍可阅读。");
      });
    return () => {
      controller.abort();
      request.current?.abort();
      requestGeneration.current++;
    };
  }, [episodeId, artifactSetId, accept]);
  const current = people?.source_version === sourceVersion;
  const draftOutdated = !!draft && (draft.outdated || draft.source_version !== sourceVersion || !current);
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
    if (operation.current) return false;
    operation.current = true;
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
      if (generation.current === version) { setBusy(""); operation.current = false; }
    }
  };
  const loadHistory = async () => {
    if (!episodeId) return;
    const version = generation.current;
    try {
      const result = await episodeCopilotApi.peopleDrafts(episodeId);
      if (generation.current === version) setHistory(result);
    } catch {
      if (generation.current === version) setError("草稿读取失败，请重试。");
    }
  };
  const reconcile = async (version: number, cancelled: boolean, previousDraft = people?.draft) => {
    if (!episodeId) return;
    operation.current = true;
    setBusy("正在核对已保存结果…");
    setProgress(null);
    const controller = new AbortController();
    request.current = controller;
    try {
      const value = await episodeCopilotApi.getPeople(episodeId, controller.signal);
      if (generation.current !== version) return;
      accept(value);
      setNeedsReadback(false);
      const newDraft = value.draft && (value.draft.id !== previousDraft?.id || value.draft.revision !== previousDraft?.revision);
      setError(newDraft || cancelled ? "" : "识别未完成或连接中断，已保存结果保留，可重试。");
      setSaved(newDraft ? "已核对：草稿已保存，等待你确认" : cancelled
        ? "已请求取消；未发现新的已保存草稿。已有结果保留。" : "");
      await loadHistory();
    } catch {
      if (generation.current === version) {
        setNeedsReadback(true);
        setError("结果状态尚未确认，请重新读取后再决定是否重试。");
      }
    } finally { if (generation.current === version) { setBusy(""); operation.current = false; } }
  };
  const prepare = async (started: number) => {
    if (!episodeId || dirty || operation.current || needsReadback) return;
    operation.current = true;
    const controller = new AbortController();
    request.current = controller;
    const version = ++generation.current;
    let streamID: string | undefined;
    setOpen(true);
    setBusy("正在识别人物…");
    setError("");
    setSaved("");
    panelScroll.current = 0;
    if (panelBodyElement.current) panelBodyElement.current.scrollTop = 0;
    setProgress({stage: "", started});
    try {
      const value = await episodeCopilotApi.preparePeople(episodeId, controller.signal, (event: PersonPreparationEvent) => {
        if (generation.current !== version || controller.signal.aborted) return;
        if (streamID && streamID !== event.request_id) return;
        streamID = event.request_id;
        if (event.source_version && event.source_version !== sourceVersion) return;
        if (event.type === "stage" && event.stage) setProgress((p) => p ? {...p, stage:event.stage!} : null);
      });
      if (generation.current !== version || controller.signal.aborted) return;
      if (value.source_version !== sourceVersion) throw new Error("逐字稿来源已变化");
      accept(value);
      setSaved("草稿已保存，等待你确认");
      await loadHistory();
    } catch {
      if (generation.current === version) await reconcile(version, controller.signal.aborted);
    } finally {
      if (generation.current === version) { setBusy(""); setProgress(null); operation.current = false; }
    }
  };
  const cancelPreparation = () => {
    if (!progress) return;
    request.current?.abort();
    void reconcile(++generation.current, true);
  };
  const review = async (apply: boolean) => {
    if (!episodeId || !draft || draftOutdated) return;
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
    if (busy || !current) return;
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
  const close = () => setOpen(false);
  const locateFromPanel = (order: number) => {
    close();
    locate(order);
  };
  const speakers = [...new Set(segments.map((s) => s.speaker))];
  const unmatched = speakers.filter((speaker) =>
    !applied.some((a) => a.speaker_label === speaker) &&
    !draft?.matches.some((m) => m.speaker_label === speaker && m.orders.length > 0),
  );
  const selectedCount = draftOutdated ? 0 : new Set(draft?.matches.filter((m) => m.selected && !m.applied)
    .flatMap((m) => m.orders) ?? []).size;
  const reload = () => {
    if (operation.current) return;
    if (dirty) { setError("草稿尚未保存，请先保存或放弃修改。"); return; }
    void reconcile(++generation.current, false);
  };
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
          onClick={() => {
            setOpen(true);
            void loadHistory();
          }}
        >
          {busy ? "查看进度" : dirty ? "继续编辑" : pending ? "继续核对" : names.length || draft ? "管理" : "识别人物"}
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
      episodeId ? createPortal(
        <div className={styles.backdrop} style={open ? undefined : { display: "none" }} onMouseDown={(e) => {
          if (e.target === e.currentTarget) close();
        }} onClick={(e) => e.stopPropagation()}>
        <section
          ref={panelElement}
          className={styles.panel}
          role="dialog"
          aria-modal={!editing}
          aria-label="人物与发言核对"
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.stopPropagation();
              close();
            }
            e.stopPropagation();
            trapTab(e);
          }}
        >
          <header>
            <div>
              <span className={styles.eyebrow}>逐字稿 · 人物管理</span>
              <h3>人物与发言</h3>
              <p>核对姓名与具体发言，确认后才会生效。</p>
            </div>
            <button type="button" aria-label="关闭人物核对" onClick={close}>
              <IconX size={18} />
            </button>
          </header>
          <div className={styles.panelBody} ref={panelBodyElement} onScroll={(event) => { panelScroll.current = event.currentTarget.scrollTop; }}>
            {error && (
              <p role="alert" className={styles.error}>
                {error}
                <button type="button" onClick={() => void reload()}>
                  重新读取
                </button>
              </p>
            )}
            {progress && <PreparationStatus stage={progress.stage} started={progress.started} />}
            <div hidden={!!progress}>
            <div className={styles.overview}>
              <span>{speakers.length} 位说话人</span>
              <span>{unmatched.length ? `${unmatched.length} 位待确认姓名` : "核对姓名与发言范围"}</span>
            </div>
            {!draft && !busy && <p>识别人物并核对发言，或手动填写姓名。确认后才会更新逐字稿。</p>}
            {!draft && <div className={styles.actions}>
              <button type="button" disabled={!!busy || dirty || needsReadback}
                onClick={(event) => void prepare(event.timeStamp)}>开始识别</button>
            </div>}
            <p role="status">{progress ? "" : busy || (dirty ? "修改尚未保存" : saved)}</p>
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
            {draftOutdated && (
              <p className={styles.error}>
                来源已变化，此记录仅供核对。请重新识别当前逐字稿。
              </p>
            )}
            {speakers.filter((speaker) => !draft?.matches.some((m) => m.speaker_label === speaker && m.orders.length)).map((speaker, i) => {
              const group = segments.filter((s) => s.speaker === speaker);
              const assignedNames = [...new Set(applied.filter((a) => a.speaker_label === speaker).map((a) => a.display_name))];
              return <section className={styles.unmatched} key={speaker}>
                <span className={styles.avatar}>{String(i + 1).padStart(2, "0")}</span>
                <div><strong>{speaker}{assignedNames.length ? ` → ${assignedNames.join("、")}` : ""}</strong> <span className={styles.badge}>{assignedNames.length ? "已应用" : "姓名待确认"}</span>
                  <p>{group.length} 段发言 · {assignedNames.length ? "可修改或解除匹配" : "尚无可靠姓名匹配"}</p></div>
                <button type="button" disabled={!!busy || dirty || !current}
                  onClick={() => editSpeaker(group[0])}>{assignedNames.length ? "修改匹配" : "填写姓名"}</button>
              </section>;
            })}
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
                  <div className={styles.matchHeader}>
                  <label className={styles.matchTitle}>
                    <input
                      type="checkbox"
                      checked={match.selected}
                      disabled={
                        !!busy || draftOutdated || !match.orders.length
                      }
                      onChange={(e) =>
                        editMatch(match.key, { selected: e.target.checked })
                      }
                    />
                    {match.speaker_label || "尚未匹配 Speaker"}
                    <span>→</span>
                    {match.display_name}
                  </label>
                  <button type="button" className={styles.editAction}
                    aria-label={`编辑匹配 ${match.display_name}`} disabled={!!busy || draftOutdated}
                    aria-expanded={editingMatch === match.key}
                    onClick={() => setEditingMatch(editingMatch === match.key ? null : match.key)}>
                    {editingMatch === match.key ? "收起编辑" : "编辑"}
                  </button>
                  </div>
                  <p>
                    {match.applied
                      ? adjusted
                        ? "曾应用，当前归属已有调整"
                        : "已应用"
                      : match.uncertain
                        ? "存在不确定项，请核对"
                        : "识别建议，尚需确认"}{" "}
                    · {match.role === "host" ? "主持人" : match.role === "guest" ? "嘉宾" : "角色待确认"} · {match.orders.length} 段
                    {group.length === match.orders.length && group.length > 0
                      ? "（该 Speaker 全部发言）"
                      : "（局部或未匹配）"}
                  </p>
                  {before.length > 0 && (
                    <p>
                      当前：{before.join("、")} → 建议：{match.display_name}
                    </p>
                  )}
                  {editingMatch === match.key && <div className={styles.fields}>
                    <label>
                      姓名
                      <input
                        aria-label={`姓名 ${match.key}`}
                        value={match.display_name}
                        disabled={!!busy || draftOutdated}
                        onChange={(e) =>
                          editMatch(match.key, { display_name: e.target.value })
                        }
                      />
                    </label>
                    <label>
                      角色
                      <select
                        value={match.role}
                        disabled={!!busy || draftOutdated}
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
                  </div>}
                  {personEvidence(match.evidence_locator).filter((e) => e.label === "发言归属依据").slice(0, 1).map((e) =>
                    <div className={styles.evidencePreview} key={e.quote}>
                      <blockquote>“{e.quote}”</blockquote>
                      <span>发言归属依据 · {e.fragment ? `片段 ${e.fragment}` : e.source}</span>
                    </div>)}
                  <EpisodePersonEvidence
                    name={match.display_name}
                    locator={match.evidence_locator}
                    onLocate={locateFromPanel}
                  />
                  <details>
                    <summary>核对匹配范围 · {match.role === "host" ? "主持人" : match.role === "guest" ? "嘉宾" : "角色待确认"} · {match.orders.length} 段</summary>
                    {group.length === 0 && (
                      <p>尚无可靠发言绑定，可在逐字稿手动编辑 Speaker。</p>
                    )}
                    {group.map((seg) => (
                      <div className={styles.fragment} key={seg.order}>
                        <label>
                          <input
                            type="checkbox"
                            checked={match.orders.includes(seg.order)}
                            disabled={!!busy || draftOutdated}
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
                        <button type="button" onClick={() => locateFromPanel(seg.order)}>
                          定位 / 试听
                        </button>
                      </div>
                    ))}
                  </details>
                  {group.length > match.orders.length && <p className={styles.exception}>另有 {group.length - match.orders.length} 段未纳入此匹配，暂不应用。</p>}
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
          </div>
          {progress ? <footer className={styles.progressFooter}>
            <span className={styles.footerSummary}>完成后由你确认，才会更新逐字稿。<small>收起弹层可继续阅读，已有结果保留。</small></span>
            <button type="button" onClick={cancelPreparation}>取消识别</button>
          </footer> : draft && (
            <footer>
              <span className={styles.footerSummary}>本次将更新 {selectedCount} 段发言<small>{dirty ? "修改尚未保存" : "草稿已保存"}</small></span>
              <button type="button" className={styles.reidentify} disabled={!!busy || dirty || needsReadback}
                onClick={(event) => void prepare(event.timeStamp)}><span aria-hidden="true">↻ </span>重新识别</button>
              <button
                type="button"
                disabled={!dirty || !!busy || draftOutdated}
                onClick={() => void review(false)}
              >
                保存草稿
              </button>
              <button
                type="button"
                className={styles.primary}
                disabled={
                  !!busy ||
                  draftOutdated ||
                  selectedCount === 0
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
        </section>
        </div>, document.body
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
