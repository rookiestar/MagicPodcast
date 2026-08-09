export type PodcastCoverLoadPriority = "high" | "medium" | "low";

interface LoadListener {
  onStart: () => void;
  onError?: () => void;
  active: boolean;
}

interface QueueEntry {
  src: string;
  priority: PodcastCoverLoadPriority;
  sequence: number;
  status: "queued" | "loading" | "loaded";
  listeners: Set<LoadListener>;
}

interface RequestOptions {
  src: string;
  priority: PodcastCoverLoadPriority;
  onStart: () => void;
  onError?: () => void;
}

export interface PodcastCoverLoadHandle {
  updatePriority: (priority: PodcastCoverLoadPriority) => void;
  release: () => void;
}

const MAX_CONCURRENT_COVERS = 4;
const MAX_BACKGROUND_COVERS = 1;

function getPriorityScore(priority: PodcastCoverLoadPriority) {
  switch (priority) {
    case "high":
      return 3;
    case "medium":
      return 2;
    case "low":
      return 1;
  }
}

/**
 * Coordinates the start budget for podcast cover elements.
 *
 * The queue only gates the real image element; it never preloads a second
 * copy with a different URL. Successful URLs remain known for this page
 * session so a virtualized remount can attach immediately without another
 * queue start.
 */
class PodcastCoverLoadQueue {
  private entries = new Map<string, QueueEntry>();
  private queuedEntries: QueueEntry[] = [];
  private activeEntries = new Set<QueueEntry>();
  private activeBackgroundCount = 0;
  private loadedSources = new Set<string>();
  private sequence = 0;

  request({ src, priority, onStart, onError }: RequestOptions) {
    const listener: LoadListener = { onStart, onError, active: true };

    if (this.loadedSources.has(src)) {
      onStart();
      return this.createHandle(undefined, listener);
    }

    let entry = this.entries.get(src);
    if (!entry) {
      entry = {
        src,
        priority,
        sequence: this.sequence++,
        status: "queued",
        listeners: new Set(),
      };
      this.entries.set(src, entry);
      this.queuedEntries.push(entry);
    } else if (
      entry.status === "queued" &&
      getPriorityScore(priority) > getPriorityScore(entry.priority)
    ) {
      entry.priority = priority;
    }

    entry.listeners.add(listener);
    if (entry.status === "loading") {
      onStart();
    } else {
      this.process();
    }

    return this.createHandle(entry, listener);
  }

  complete(src: string) {
    const entry = this.entries.get(src);
    if (!entry || entry.status === "loaded") {
      return;
    }

    if (entry.status === "queued") {
      this.queuedEntries = this.queuedEntries.filter(
        (queuedEntry) => queuedEntry !== entry,
      );
      entry.status = "loaded";
      this.loadedSources.add(src);
      return;
    }

    entry.status = "loaded";
    this.activeEntries.delete(entry);
    if (entry.priority === "low") {
      this.activeBackgroundCount -= 1;
    }
    this.loadedSources.add(src);
    this.process();
  }

  fail(src: string) {
    const entry = this.entries.get(src);
    if (!entry || entry.status !== "loading") {
      return false;
    }

    this.activeEntries.delete(entry);
    if (entry.priority === "low") {
      this.activeBackgroundCount -= 1;
    }
    this.entries.delete(src);

    const listeners = [...entry.listeners];
    entry.listeners.clear();
    for (const listener of listeners) {
      if (listener.active) {
        listener.active = false;
        listener.onError?.();
      }
    }
    this.process();
    return true;
  }

  clear() {
    this.entries.clear();
    this.queuedEntries = [];
    this.activeEntries.clear();
    this.activeBackgroundCount = 0;
    this.loadedSources.clear();
    this.sequence = 0;
  }

  getStatus() {
    return {
      queued: this.queuedEntries.length,
      active: this.activeEntries.size,
      loaded: this.loadedSources.size,
      backgroundActive: this.activeBackgroundCount,
    };
  }

  private createHandle(
    entry: QueueEntry | undefined,
    listener: LoadListener,
  ): PodcastCoverLoadHandle {
    return {
      updatePriority: (priority) => {
        if (
          !entry ||
          !listener.active ||
          entry.status !== "queued" ||
          getPriorityScore(priority) <= getPriorityScore(entry.priority)
        ) {
          return;
        }

        entry.priority = priority;
        this.process();
      },
      release: () => {
        if (!entry || !listener.active) {
          return;
        }

        listener.active = false;
        entry.listeners.delete(listener);

        if (entry.listeners.size > 0) {
          return;
        }

        if (entry.status === "queued") {
          this.entries.delete(entry.src);
          this.queuedEntries = this.queuedEntries.filter(
            (queuedEntry) => queuedEntry !== entry,
          );
          return;
        }

        if (entry.status === "loading") {
          this.entries.delete(entry.src);
          this.activeEntries.delete(entry);
          if (entry.priority === "low") {
            this.activeBackgroundCount -= 1;
          }
          this.process();
        }
      },
    };
  }

  private process() {
    this.queuedEntries.sort(
      (first, second) =>
        getPriorityScore(second.priority) -
          getPriorityScore(first.priority) ||
        first.sequence - second.sequence,
    );

    while (this.activeEntries.size < MAX_CONCURRENT_COVERS) {
      const nextIndex = this.queuedEntries.findIndex(
        (entry) =>
          entry.priority !== "low" ||
          this.activeBackgroundCount < MAX_BACKGROUND_COVERS,
      );

      if (nextIndex < 0) {
        return;
      }

      const [entry] = this.queuedEntries.splice(nextIndex, 1);
      entry.status = "loading";
      this.activeEntries.add(entry);
      if (entry.priority === "low") {
        this.activeBackgroundCount += 1;
      }

      for (const listener of entry.listeners) {
        if (listener.active) {
          listener.onStart();
        }
      }
    }
  }
}

export const podcastCoverLoadQueue = new PodcastCoverLoadQueue();
