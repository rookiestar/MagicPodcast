import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  isLogContainerNearBottom,
  useStableLogScroll,
} from "../useStableLogScroll";

function setScrollMetrics(
  element: HTMLElement,
  {
    clientHeight,
    scrollHeight,
    scrollTop,
  }: { clientHeight: number; scrollHeight: number; scrollTop: number },
) {
  Object.defineProperty(element, "clientHeight", {
    value: clientHeight,
    configurable: true,
  });
  Object.defineProperty(element, "scrollHeight", {
    value: scrollHeight,
    configurable: true,
  });
  Object.defineProperty(element, "scrollTop", {
    value: scrollTop,
    writable: true,
    configurable: true,
  });
}

function LogScrollHarness() {
  const [itemCount, setItemCount] = useState(2);
  const {
    autoScroll,
    logContainerRef,
    logEndRef,
    handleLogScroll,
    resumeAutoScroll,
  } = useStableLogScroll(itemCount);

  return (
    <div>
      <button type="button" onClick={() => setItemCount((count) => count + 1)}>
        append
      </button>
      <button type="button" onClick={resumeAutoScroll}>
        resume
      </button>
      <div aria-label="logs" ref={logContainerRef} onScroll={handleLogScroll}>
        {Array.from({ length: itemCount }, (_, index) => (
          <p key={index}>log {index + 1}</p>
        ))}
        <div ref={logEndRef} />
      </div>
      <span>{autoScroll ? "auto" : "manual"}</span>
    </div>
  );
}

describe("useStableLogScroll", () => {
  beforeEach(() => {
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      () => {},
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("detects whether the log container is near the bottom", () => {
    expect(
      isLogContainerNearBottom({
        clientHeight: 200,
        scrollHeight: 800,
        scrollTop: 580,
      }),
    ).toBe(true);
    expect(
      isLogContainerNearBottom({
        clientHeight: 200,
        scrollHeight: 800,
        scrollTop: 320,
      }),
    ).toBe(false);
  });

  it("keeps manual scroll position stable when new items arrive", async () => {
    render(<LogScrollHarness />);

    const logContainer = screen.getByLabelText("logs");
    setScrollMetrics(logContainer, {
      clientHeight: 200,
      scrollHeight: 800,
      scrollTop: 160,
    });

    fireEvent.scroll(logContainer);

    await waitFor(() => {
      expect(screen.getByText("manual")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "append" }));

    await waitFor(() => {
      expect(screen.getByText("log 3")).toBeInTheDocument();
    });
    expect(logContainer.scrollTop).toBe(160);
  });
});
