import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useInfiniteScrollTrigger } from "../usePagination";

class MockIntersectionObserver {
  static instances: MockIntersectionObserver[] = [];

  callback: IntersectionObserverCallback;
  options?: IntersectionObserverInit;
  observe = vi.fn();
  disconnect = vi.fn(() => {
    this.disconnected = true;
  });
  disconnected = false;

  constructor(
    callback: IntersectionObserverCallback,
    options?: IntersectionObserverInit,
  ) {
    this.callback = callback;
    this.options = options;
    MockIntersectionObserver.instances.push(this);
  }

  trigger(isIntersecting: boolean) {
    if (this.disconnected) {
      return;
    }

    this.callback(
      [{ isIntersecting } as IntersectionObserverEntry],
      this as unknown as IntersectionObserver,
    );
  }
}

describe("useInfiniteScrollTrigger", () => {
  beforeEach(() => {
    MockIntersectionObserver.instances = [];
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("runs the callback only when the observed element intersects", () => {
    const callback = vi.fn();
    const element = document.createElement("div");

    const { result } = renderHook(() =>
      useInfiniteScrollTrigger(callback, { rootMargin: "300px" }),
    );

    act(() => {
      result.current.ref(element);
    });

    const [observer] = MockIntersectionObserver.instances;
    expect(observer.observe).toHaveBeenCalledWith(element);
    expect(observer.options).toMatchObject({
      root: null,
      rootMargin: "300px",
      threshold: 0.1,
    });

    act(() => {
      observer.trigger(false);
      observer.trigger(true);
    });

    expect(callback).toHaveBeenCalledTimes(1);
  });

  it("keeps the same observer when only the callback changes", () => {
    const firstCallback = vi.fn();
    const secondCallback = vi.fn();
    const element = document.createElement("div");

    const { result, rerender } = renderHook(
      ({ callback }) => useInfiniteScrollTrigger(callback),
      { initialProps: { callback: firstCallback } },
    );

    act(() => {
      result.current.ref(element);
    });

    const [observer] = MockIntersectionObserver.instances;

    rerender({ callback: secondCallback });

    expect(MockIntersectionObserver.instances).toHaveLength(1);
    expect(observer.disconnect).not.toHaveBeenCalled();

    act(() => {
      observer.trigger(true);
    });

    expect(firstCallback).not.toHaveBeenCalled();
    expect(secondCallback).toHaveBeenCalledTimes(1);
  });

  it("does not observe while disabled and reconnects when enabled", () => {
    const callback = vi.fn();
    const element = document.createElement("div");

    const { result, rerender } = renderHook(
      ({ enabled }) => useInfiniteScrollTrigger(callback, { enabled }),
      { initialProps: { enabled: false } },
    );

    act(() => {
      result.current.ref(element);
    });

    expect(MockIntersectionObserver.instances).toHaveLength(0);

    rerender({ enabled: true });

    expect(MockIntersectionObserver.instances).toHaveLength(1);
    expect(MockIntersectionObserver.instances[0].observe).toHaveBeenCalledWith(
      element,
    );

    rerender({ enabled: false });

    expect(MockIntersectionObserver.instances[0].disconnect).toHaveBeenCalled();

    act(() => {
      MockIntersectionObserver.instances[0].trigger(true);
    });

    expect(callback).not.toHaveBeenCalled();
  });

  it("falls back to scroll detection when IntersectionObserver is unavailable", () => {
    vi.stubGlobal("IntersectionObserver", undefined);

    const callback = vi.fn();
    const element = document.createElement("div");
    let top = 1000;

    Object.defineProperty(window, "innerHeight", {
      configurable: true,
      value: 400,
    });
    element.getBoundingClientRect = vi.fn(() => ({
      top,
      bottom: top + 50,
      left: 0,
      right: 100,
      width: 100,
      height: 50,
      x: 0,
      y: top,
      toJSON: () => ({}),
    }));

    const { result } = renderHook(() =>
      useInfiniteScrollTrigger(callback, { rootMargin: "300px" }),
    );

    act(() => {
      result.current.ref(element);
    });

    expect(callback).not.toHaveBeenCalled();

    top = 650;
    act(() => {
      window.dispatchEvent(new Event("scroll"));
      window.dispatchEvent(new Event("scroll"));
    });

    expect(callback).toHaveBeenCalledTimes(1);
  });
});
