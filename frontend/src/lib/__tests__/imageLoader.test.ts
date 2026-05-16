import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { imageLoadQueue } from "../imageLoader";

class FakeImage {
  static instances: FakeImage[] = [];

  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  src = "";

  constructor() {
    FakeImage.instances.push(this);
  }
}

function createTask(overrides = {}) {
  return {
    id: "image-1",
    src: "https://example.com/image.jpg",
    imgElement: document.createElement("img"),
    priority: "high" as const,
    retryCount: 0,
    onSuccess: vi.fn(),
    onError: vi.fn(),
    ...overrides,
  };
}

describe("imageLoadQueue", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal("Image", FakeImage);
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    vi.spyOn(console, "log").mockImplementation(() => undefined);
    vi.spyOn(console, "warn").mockImplementation(() => undefined);
    FakeImage.instances = [];
    imageLoadQueue.clear();
  });

  afterEach(() => {
    imageLoadQueue.clear();
    vi.restoreAllMocks();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("does not fire callbacks after an active image is cancelled", () => {
    const task = createTask();

    imageLoadQueue.add(task);
    expect(imageLoadQueue.getStatus().loading).toBe(1);

    imageLoadQueue.cancel(task.id);
    FakeImage.instances[0].onload?.();

    expect(task.onSuccess).not.toHaveBeenCalled();
    expect(task.onError).not.toHaveBeenCalled();
    expect(task.imgElement.src).toBe("");
    expect(imageLoadQueue.getStatus().loading).toBe(0);
  });

  it("ignores a late success after a timed out attempt is retried", () => {
    const task = createTask();

    imageLoadQueue.add(task);
    const firstAttempt = FakeImage.instances[0];

    vi.advanceTimersByTime(5000);
    firstAttempt.onload?.();

    expect(task.onSuccess).not.toHaveBeenCalled();
    expect(task.imgElement.src).toBe("");
    expect(FakeImage.instances).toHaveLength(2);

    FakeImage.instances[1].onload?.();

    expect(task.onSuccess).toHaveBeenCalledTimes(1);
    expect(task.imgElement.src).toBe(task.src);
  });

  it("clears queued and active work", () => {
    const firstTask = createTask({ id: "image-1" });
    const secondTask = createTask({ id: "image-2" });
    const thirdTask = createTask({ id: "image-3" });
    const fourthTask = createTask({ id: "image-4" });

    imageLoadQueue.add(firstTask);
    imageLoadQueue.add(secondTask);
    imageLoadQueue.add(thirdTask);
    imageLoadQueue.add(fourthTask);

    expect(imageLoadQueue.getStatus()).toMatchObject({
      loading: 3,
      queue: 1,
    });

    imageLoadQueue.clear();
    FakeImage.instances.forEach((image) => image.onload?.());

    expect(firstTask.onSuccess).not.toHaveBeenCalled();
    expect(secondTask.onSuccess).not.toHaveBeenCalled();
    expect(thirdTask.onSuccess).not.toHaveBeenCalled();
    expect(fourthTask.onSuccess).not.toHaveBeenCalled();
    expect(imageLoadQueue.getStatus()).toMatchObject({
      loading: 0,
      queue: 0,
    });
  });
});
