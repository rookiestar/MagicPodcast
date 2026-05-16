import { describe, it, expect } from "vitest";
import { stripHtml, truncateText } from "../textUtils";

// 简单的文本处理工具函数测试
describe("文本工具函数", () => {
  describe("stripHtml", () => {
    it("应该提取 HTML 中的纯文本并清理空白", () => {
      expect(stripHtml("<p>第一段</p><p>第二段</p>")).toBe("第一段第二段");
      expect(stripHtml("  第一段\n\n第二段  ")).toBe("第一段 第二段");
    });

    it("应该移除脚本和样式内容", () => {
      expect(
        stripHtml(
          '<p>正文</p><script>alert("x")</script><style>.x{}</style>',
        ),
      ).toBe("正文");
    });

    it("应该按长度截断", () => {
      expect(stripHtml("<p>abcdefghijklmnopqrstuvwxyz</p>", 5)).toBe("abcde...");
    });
  });

  describe("truncateText", () => {
    it("应该正确处理短文本和长文本", () => {
      expect(truncateText("短文本", 10)).toBe("短文本");
      expect(truncateText("abcdefghijklmnopqrstuvwxyz", 5)).toBe("abcde...");
    });
  });

  describe("字符串处理", () => {
    it("应该正确连接字符串", () => {
      const str1 = "Hello";
      const str2 = "World";
      const result = `${str1} ${str2}`;
      expect(result).toBe("Hello World");
    });

    it("应该正确截断字符串", () => {
      const longText = "This is a very long text that needs to be truncated";
      const maxLength = 20;
      const truncated = longText.substring(0, maxLength) + "...";
      expect(truncated.length).toBeLessThanOrEqual(maxLength + 3);
    });

    it("应该正确判断空字符串", () => {
      expect("".trim()).toBe("");
      expect("  ".trim()).toBe("");
    });
  });

  describe("数组处理", () => {
    it("应该正确过滤数组", () => {
      const arr = [1, 2, 3, 4, 5];
      const filtered = arr.filter((x) => x > 2);
      expect(filtered).toEqual([3, 4, 5]);
    });

    it("应该正确映射数组", () => {
      const arr = [1, 2, 3];
      const mapped = arr.map((x) => x * 2);
      expect(mapped).toEqual([2, 4, 6]);
    });
  });

  describe("数字处理", () => {
    it("应该正确格式化数字", () => {
      const num = 1234.5678;
      const formatted = num.toFixed(2);
      expect(formatted).toBe("1234.57");
    });

    it("应该正确四舍五入", () => {
      expect(Math.round(1.4)).toBe(1);
      expect(Math.round(1.5)).toBe(2);
    });
  });
});
