"use client";

import { useEffect, useSyncExternalStore } from "react";
import { usePathname } from "next/navigation";
import { singleParam, positiveID, episodeIDFromHref } from "./navigationParams";
export { singleParam, positiveID, episodeIDFromHref, episodeHref } from "./navigationParams";


const CHANGE = "magicpodcast-navigation";
const BEFORE = "magicpodcast-before-navigation";
const STATE = "magicpodcastNavigation";

type Entry = { index: number; href: string; previous?: string };
const locationHref = () =>
  window.location.pathname + window.location.search + window.location.hash;
const serverHref = () => "";
let current: Entry | undefined;
let restoring = false;
let approvedPopHref: string | undefined;

function historyData(entry: Entry) {
  const data = { ...window.history.state, [STATE]: entry };
  // Next copies its private flags itself. Passing them here bypasses its
  // native-history integration and leaves usePathname/useSearchParams stale.
  delete data.__NA;
  delete data._N;
  return data;
}

function approve(href: string) {
  return window.dispatchEvent(
    new CustomEvent(BEFORE, { cancelable: true, detail: href }),
  );
}
function subscribe(notify: () => void) {
  const changed = () => notify();
  window.addEventListener(CHANGE, changed);
  window.addEventListener("popstate", changed);
  return () => {
    window.removeEventListener(CHANGE, changed);
    window.removeEventListener("popstate", changed);
  };
}

// Install before Next's listener can render a different tree on a rejected back.
function guardPopState(event: PopStateEvent) {
  const next: Entry | undefined = event.state?.[STATE];
  if (restoring) {
    restoring = false;
    current = next;
    window.dispatchEvent(new Event(CHANGE));
    return;
  }
  const preapproved = approvedPopHref === locationHref();
  approvedPopHref = undefined;
  if (current && !preapproved && !approve(locationHref())) {
    event.stopImmediatePropagation();
    const distance = current.index - (next?.index ?? 0);
    if (distance) {
      restoring = true;
      window.history.go(distance);
    } else {
      window.history.replaceState(historyData(current), "", current.href);
    }
    return;
  }
  current = next;
}
if (typeof window !== "undefined")
  window.addEventListener("popstate", guardPopState, true);

export function useLocationHref() {
  // Framework route transitions do not emit popstate or our native-history event.
  const pathname = usePathname();
  useEffect(() => { window.dispatchEvent(new Event(CHANGE)); }, [pathname]);
  return useSyncExternalStore(subscribe, locationHref, serverHref);
}

export function navigate(href: string, options: { replace?: boolean } = {}) {
  const url = new URL(href, window.location.origin);
  if (url.origin !== window.location.origin)
    throw new Error("Navigation must remain on this site");
  const nextHref = url.pathname + url.search + url.hash;
  if (nextHref === locationHref()) return true;
  if (!approve(nextHref)) return false;
  const previous: Entry = window.history.state?.[STATE] ?? {
    index: 0,
    href: locationHref(),
  };
  if (!window.history.state?.[STATE] && !options.replace) {
    window.history.replaceState(historyData(previous), "");
  }
  current = {
    index: previous.index + (options.replace ? 0 : 1),
    href: nextHref,
    previous: options.replace ? previous.previous : previous.href,
  };
  const nextState = historyData(current);
  const inheritedOrigin = window.history.state?.[ORIGIN] as EpisodeOrigin | undefined;
  const nextEpisodeID = episodeIDFromHref(nextHref);
  if (nextEpisodeID && episodeIDFromHref(locationHref()) && inheritedOrigin && isListOrigin(inheritedOrigin.href)) {
    nextState[ORIGIN] = { ...inheritedOrigin, episodeID: nextEpisodeID };
  } else {
    delete nextState[ORIGIN];
  }
  window.history[options.replace ? "replaceState" : "pushState"](
    nextState,
    "",
    nextHref,
  );
  window.dispatchEvent(new Event(CHANGE));
  return true;
}
export function updateQuery(
  values: Record<string, string | string[] | null>,
  replace = false,
) {
  const url = new URL(window.location.href);
  for (const [key, value] of Object.entries(values)) {
    url.searchParams.delete(key);
    for (const item of Array.isArray(value)
      ? value
      : value === null
        ? []
        : [value]) {
      if (item !== "") url.searchParams.append(key, item);
    }
  }
  return navigate(url.pathname + url.search + url.hash, { replace });
}
export function closeTo(href: string) {
  const entry: Entry | undefined = window.history.state?.[STATE];
  if (entry?.previous === href) {
    if (!approve(href)) return false;
    approvedPopHref = href;
    window.history.back();
    return true;
  }
  return navigate(href, { replace: true });
}

/** Guard only navigation that would discard this editor; queries can preserve it. */
export function useUnsavedNavigation(
  dirty: boolean,
  retains: (href: string) => boolean,
) {
  useEffect(() => {
    if (!dirty) return;
    const savedEntry: Entry | undefined = window.history.state?.[STATE];
    current = savedEntry?.href === locationHref() ? savedEntry : { index: savedEntry?.index ?? 0, href: locationHref() };
    if (!savedEntry || savedEntry.href !== current.href) window.history.replaceState(historyData(current), "");
    const before = (event: Event) => {
      const target = (event as CustomEvent<string>).detail;
      if (
        !retains(target) &&
        !window.confirm("有未保存的修改，确定离开并放弃修改？")
      )
        event.preventDefault();
    };
    const unload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const click = (event: MouseEvent) => {
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
      )
        return;
      const anchor = (event.target as Element).closest?.("a[href]");
      if (!(anchor instanceof HTMLAnchorElement) || anchor.target === "_blank")
        return;
      const url = new URL(anchor.href);
      const target = url.pathname + url.search + url.hash;
      if (
        (url.origin !== window.location.origin || !retains(target)) &&
        !window.confirm("有未保存的修改，确定离开并放弃修改？")
      ) {
        event.preventDefault();
        event.stopImmediatePropagation();
      }
    };
    document.addEventListener("click", click, true);
    window.addEventListener(BEFORE, before);
    window.addEventListener("beforeunload", unload);
    return () => {
      document.removeEventListener("click", click, true);
      window.removeEventListener(BEFORE, before);
      window.removeEventListener("beforeunload", unload);
    };
  }, [dirty, retains]);
}

export function parseEpisodeRoute(href: string) {
  const url = new URL(href || "/", "http://navigation.local");
  const params = url.searchParams;
  const tab = singleParam(params, "tab");
  const artifact = singleParam(params, "artifact");
  const source = singleParam(params, "source");
  const sourceID = source?.startsWith("artifact-") ? positiveID(source.slice(9)) : null;
  const fragment = positiveID(singleParam(params, "fragment"));
  const seconds = singleParam(params, "t");
  const time = seconds !== null && seconds.trim() !== "" && Number.isFinite(Number(seconds)) && Number(seconds) >= 0 ? Number(seconds) : null;
  const hasReference = params.has("source") || params.has("fragment");
  const referenceInvalid = hasReference && (!sourceID || (params.has("fragment") && !fragment));
  return {
    source, sourceID, fragment, hasReference, referenceInvalid,
    time, timeInvalid: params.has("t") && time === null,
    peopleOpen: singleParam(params, "panel") === "people",
    person: positiveID(singleParam(params, "person")),
    assistant: singleParam(params, "assistant") === "1",
    profile: singleParam(params, "profile"),
    targetPerson: positiveID(singleParam(params, "target_person")),
    targetInvalid: params.has("target_person") && !positiveID(singleParam(params, "target_person")),
    id: episodeIDFromHref(href),
    tab:
      tab === "transcript" || tab === "notes" ? tab : ("show-notes" as const),
    artifact:
      artifact === "summary" ||
      artifact === "minutes" ||
      artifact === "transcript"
        ? artifact
        : undefined,
  } as const;
}

export function normalizeEpisodeQuery() {
  const params = new URLSearchParams(window.location.search);
  const patch: Record<string, string | null> = {};
  for (const [key, allowed] of Object.entries({
    tab: ["show-notes", "transcript", "notes"],
    artifact: ["summary", "minutes", "transcript"],
    panel: ["people"], assistant: ["1"],
  })) {
    if (params.has(key) && !allowed.includes(singleParam(params, key) ?? ""))
      patch[key] = null;
  }
  if (singleParam(params, "tab") === "show-notes") patch.tab = null;
  if (singleParam(params, "panel") === "people" || params.has("source") || params.has("fragment") || params.has("t")) {
    if (singleParam(params, "tab") !== "transcript") patch.tab = "transcript";
    if (singleParam(params, "artifact") !== "transcript") patch.artifact = "transcript";
  }
  if (params.has("fragment") && params.has("t")) patch.t = null;
  if (Object.keys(patch).length) updateQuery(patch, true);
}

export type EpisodeRoute = ReturnType<typeof parseEpisodeRoute>;


export type EpisodeOrigin = { episodeID: number; href: string; scrollY: number; triggerID: string; index: number; containers?: Array<{ levels: number; top: number; left: number }> };
const ORIGIN = "magicpodcastEpisodeOrigin";
let pendingEpisodeOrigin: EpisodeOrigin | undefined;
let pendingEpisodeReturn: EpisodeOrigin | undefined;

function isListOrigin(href: string) {
  return /^\/(?:inbox(?:\/history)?|discovery|search|podcasts(?:\/[1-9]\d*)?|workflows(?:\/[1-9]\d*(?:\/reports\/[1-9]\d*)?)?)(?:[?#]|$)/.test(href);
}

export function rememberEpisodeOrigin(episodeID: number, triggerID: string) {
  const href = locationHref();
  if (!isListOrigin(href)) return;
  const entry: Entry = window.history.state?.[STATE] ?? { index: 0, href };
  window.history.replaceState(historyData(entry), "");
  const containers: NonNullable<EpisodeOrigin["containers"]> = [];
  let parent = document.getElementById(triggerID)?.parentElement;
  for (let levels = 1; parent && parent !== document.body; levels++, parent = parent.parentElement) {
    if (parent.scrollTop || parent.scrollLeft) containers.push({ levels, top: parent.scrollTop, left: parent.scrollLeft });
  }
  pendingEpisodeOrigin = { episodeID, href, scrollY: window.scrollY, triggerID, index: entry.index, containers };
}

export function attachEpisodeOrigin(episodeID: number | null) {
  if (!pendingEpisodeOrigin || pendingEpisodeOrigin.episodeID !== episodeID) return;
  const origin = pendingEpisodeOrigin;
  pendingEpisodeOrigin = undefined;
  current = { index: origin.index + 1, href: locationHref(), previous: origin.href };
  window.history.replaceState({ ...historyData(current), [ORIGIN]: origin }, "");
}

export function readEpisodeOrigin(episodeID: number | null): EpisodeOrigin | undefined {
  const origin = window.history.state?.[ORIGIN] as EpisodeOrigin | undefined;
  return origin?.episodeID === episodeID && isListOrigin(origin.href) ? origin : undefined;
}

export function episodeOriginIsPrevious(origin: EpisodeOrigin) {
  return window.history.state?.[STATE]?.previous === origin.href;
}

export function queueEpisodeReturn(origin: EpisodeOrigin) {
  pendingEpisodeReturn = origin;
}

export function useEpisodeReturnRestoration() {
  const href = useLocationHref();
  useEffect(() => {
    const origin = pendingEpisodeReturn;
    if (!origin || href !== origin.href) return;
    let timer: number | undefined;
    let frame: number | undefined;
    let scheduled = false;
    const restore = () => {
      const trigger = document.getElementById(origin.triggerID);
      if (!trigger) return false;
      if (scheduled) return true;
      scheduled = true;
      // Let the restored page mount its own dialogs before restoring the trigger.
      frame = requestAnimationFrame(() => {
        frame = requestAnimationFrame(() => {
          if (locationHref() !== origin.href || !trigger.isConnected) return;
          window.scrollTo(0, origin.scrollY);
          for (const saved of origin.containers ?? []) {
            let container: HTMLElement | null = trigger;
            for (let level = 0; level < saved.levels; level++) container = container?.parentElement ?? null;
            if (container) { container.scrollTop = saved.top; container.scrollLeft = saved.left; }
          }
          trigger.focus({ preventScroll: true });
          pendingEpisodeReturn = undefined;
        });
      });
      if (timer !== undefined) window.clearTimeout(timer);
      return true;
    };
    if (restore()) return () => { if (frame !== undefined) cancelAnimationFrame(frame); };
    const observer = new MutationObserver(() => { if (restore()) observer.disconnect(); });
    observer.observe(document.body, { childList: true, subtree: true });
    timer = window.setTimeout(() => {
      observer.disconnect();
      window.scrollTo(0, origin.scrollY);
      pendingEpisodeReturn = undefined;
    }, 5000);
    return () => { observer.disconnect(); window.clearTimeout(timer); if (frame !== undefined) cancelAnimationFrame(frame); };
  }, [href]);
}
