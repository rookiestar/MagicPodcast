import { describe, expect, it } from "vitest";
import { metadata } from "../layout";

describe("document metadata", () => {
  it("uses the knowledge-library title and the editorial tuning mark favicon", () => {
    expect(metadata.title).toBe("MagicPodcast - 个人播客知识库");
    expect(metadata.icons).toMatchObject({
      icon: {
        url: "/brand/magicpodcast-tuning-mark.png",
        type: "image/png",
      },
      apple: "/brand/magicpodcast-tuning-mark.png",
    });
  });
});
