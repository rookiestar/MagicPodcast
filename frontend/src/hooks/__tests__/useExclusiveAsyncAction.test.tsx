import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { useExclusiveAsyncAction } from "../useExclusiveAsyncAction";

function deferred<T = void>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => {};
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });

  return { promise, resolve };
}

function ExclusiveActionHarness({ action }: { action: () => Promise<void> }) {
  const [blocked, setBlocked] = useState(false);
  const runExclusiveAction = useExclusiveAsyncAction({ isBlocked: blocked });

  return (
    <div>
      <button
        type="button"
        onClick={() => {
          void runExclusiveAction(action);
        }}
      >
        run
      </button>
      <button type="button" onClick={() => setBlocked(true)}>
        block
      </button>
    </div>
  );
}

describe("useExclusiveAsyncAction", () => {
  it("ignores reentry while an action is still running", async () => {
    const run = deferred();
    const action = vi.fn(() => run.promise);

    render(<ExclusiveActionHarness action={action} />);

    const runButton = screen.getByRole("button", { name: "run" });
    fireEvent.click(runButton);
    fireEvent.click(runButton);

    expect(action).toHaveBeenCalledTimes(1);

    await act(async () => {
      run.resolve();
      await run.promise;
    });

    fireEvent.click(runButton);
    expect(action).toHaveBeenCalledTimes(2);
  });

  it("does not run when the action is blocked", () => {
    const action = vi.fn(() => Promise.resolve());

    render(<ExclusiveActionHarness action={action} />);

    fireEvent.click(screen.getByRole("button", { name: "block" }));
    fireEvent.click(screen.getByRole("button", { name: "run" }));

    expect(action).not.toHaveBeenCalled();
  });
});
