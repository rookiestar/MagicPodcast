import { afterEach, describe, expect, it } from "vitest";
import { acquireDocumentScrollLock } from "../documentScrollLock";

describe("acquireDocumentScrollLock", () => {
  afterEach(() => {
    document.body.style.overflow = "";
    document.body.style.touchAction = "";
    document.documentElement.style.overflow = "";
  });

  it("restores original styles after nested locks release parent first", () => {
    document.body.style.overflow = "scroll";
    document.body.style.touchAction = "pan-y";
    document.documentElement.style.overflow = "auto";

    const releaseOuter = acquireDocumentScrollLock();
    const releaseInner = acquireDocumentScrollLock({
      lockDocumentElement: true,
      disableTouchAction: true,
    });

    releaseOuter();
    expect(document.body.style.overflow).toBe("hidden");
    expect(document.body.style.touchAction).toBe("none");
    expect(document.documentElement.style.overflow).toBe("hidden");

    releaseInner();
    expect(document.body.style.overflow).toBe("scroll");
    expect(document.body.style.touchAction).toBe("pan-y");
    expect(document.documentElement.style.overflow).toBe("auto");

    releaseInner();
    expect(document.body.style.overflow).toBe("scroll");
  });
});
