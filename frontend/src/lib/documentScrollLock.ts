interface StyleLockState {
  count: number;
  originalValue: string;
}

interface DocumentScrollLockOptions {
  lockDocumentElement?: boolean;
  disableTouchAction?: boolean;
}

const bodyOverflowLock: StyleLockState = { count: 0, originalValue: "" };
const documentOverflowLock: StyleLockState = { count: 0, originalValue: "" };
const bodyTouchActionLock: StyleLockState = { count: 0, originalValue: "" };

export function acquireDocumentScrollLock(
  options: DocumentScrollLockOptions = {},
) {
  const releases = [
    acquireStyleLock(
      () => document.body.style.overflow,
      (value) => {
        document.body.style.overflow = value;
      },
      "hidden",
      bodyOverflowLock,
    ),
  ];
  if (options.lockDocumentElement) {
    releases.push(
      acquireStyleLock(
        () => document.documentElement.style.overflow,
        (value) => {
          document.documentElement.style.overflow = value;
        },
        "hidden",
        documentOverflowLock,
      ),
    );
  }
  if (options.disableTouchAction) {
    releases.push(
      acquireStyleLock(
        () => document.body.style.touchAction,
        (value) => {
          document.body.style.touchAction = value;
        },
        "none",
        bodyTouchActionLock,
      ),
    );
  }

  return () => {
    for (let index = releases.length - 1; index >= 0; index -= 1) {
      releases[index]();
    }
  };
}

function acquireStyleLock(
  read: () => string,
  write: (value: string) => void,
  lockedValue: string,
  state: StyleLockState,
) {
  if (state.count === 0) {
    state.originalValue = read();
    write(lockedValue);
  }
  state.count += 1;
  let released = false;

  return () => {
    if (released) return;
    released = true;
    state.count -= 1;
    if (state.count === 0) {
      write(state.originalValue);
      state.originalValue = "";
    }
  };
}
