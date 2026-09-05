import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import postcss from "postcss";
import { describe, expect, it } from "vitest";

const cssRoot = postcss.parse(
  readFileSync(resolve("src/components/inbox/InboxPage.module.css"), "utf8"),
  { from: "src/components/inbox/InboxPage.module.css" },
);

describe("InboxPage overlay styles", () => {
  it("keeps the minutes viewer inside device safe areas at every width", () => {
    let baseLightboxPadding = "";
    let mobileLightboxPadding = "";
    let cardWidth = "";
    let cardHeight = "";
    cssRoot.walkRules((rule) => {
      if (rule.parent !== cssRoot || !rule.selectors.includes(".minutesLightbox")) {
        return;
      }
      rule.walkDecls("padding", (declaration) => {
        baseLightboxPadding = declaration.value;
      });
    });
    cssRoot.walkAtRules("media", (mediaRule) => {
      if (!mediaRule.params.includes("max-width: 430px")) return;
      mediaRule.walkRules((rule) => {
        rule.walkDecls((declaration) => {
          if (rule.selectors.includes(".minutesLightbox") && declaration.prop === "padding") {
            mobileLightboxPadding = declaration.value;
          }
          if (rule.selectors.includes(".minutesLightboxCard")) {
            if (declaration.prop === "width") cardWidth = declaration.value;
            if (declaration.prop === "height") cardHeight = declaration.value;
          }
        });
      });
    });

    for (const lightboxPadding of [baseLightboxPadding, mobileLightboxPadding]) {
      expect(lightboxPadding).toContain("env(safe-area-inset-top");
      expect(lightboxPadding).toContain("env(safe-area-inset-right");
      expect(lightboxPadding).toContain("env(safe-area-inset-bottom");
      expect(lightboxPadding).toContain("env(safe-area-inset-left");
    }
    expect(cardWidth).toBe("100%");
    expect(cardHeight).toBe("100%");
  });
});
