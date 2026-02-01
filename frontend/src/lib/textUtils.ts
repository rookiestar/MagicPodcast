/**
 * 文本处理工具函数
 */

/**
 * 从 HTML 文本中提取纯文本（用于列表预览）
 * @param html - HTML 字符串
 * @param maxLength - 最大长度（默认 100）
 * @returns 纯文本，超过长度会截断并添加省略号
 */
export function stripHtml(html: string, maxLength: number = 100): string {
  if (!html) return "";

  // 创建一个临时 div 元素来解析 HTML
  const tmp = document.createElement("div");
  tmp.innerHTML = html;

  // 获取纯文本内容
  let text = tmp.textContent || tmp.innerText || "";

  // 清理多余的空白字符
  text = text
    .replace(/\s+/g, " ") // 多个空白字符替换为单个空格
    .trim();

  // 截断文本
  if (text.length > maxLength) {
    text = text.substring(0, maxLength) + "...";
  }

  return text;
}

/**
 * 截断文本到指定长度
 * @param text - 原始文本
 * @param maxLength - 最大长度
 * @returns 截断后的文本
 */
export function truncateText(text: string, maxLength: number): string {
  if (!text) return "";
  if (text.length <= maxLength) return text;
  return text.substring(0, maxLength) + "...";
}
