import { describe, expect, it } from "vitest";
import { stripReportSystemMetadata } from "@/lib/reportStats";

describe("stripReportSystemMetadata", () => {
  it("removes identified system metadata and keeps ordinary quotes", () => {
    const content = [
      "# 标题",
      "",
      "> **🕐 执行**: 2026-09-10 | **⏱️ 窗口**: 7天 (cron) | **📊 统计**: 3节目/3单集",
      "",
      "> **📡 Feed覆盖**: 3/3 已尝试 | 3 成功",
      "",
      "> 用户普通引用块",
      "",
      "单集详情",
    ].join("\n");

    const stripped = stripReportSystemMetadata(content);
    expect(stripped).not.toContain("🕐 执行");
    expect(stripped).not.toContain("📡 Feed覆盖");
    expect(stripped).toContain("> 用户普通引用块");
    expect(stripped).toContain("单集详情");
  });
});
