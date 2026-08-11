"use client";

import { memo, useMemo } from "react";
import { sanitizeRichTextHtml } from "@/lib/contentSanitizer";
import { optimizeHtmlImageSources } from "@/lib/imageOptimization";
import {
  getRichTextClassName,
  type RichTextDensity,
} from "@/lib/typography";

interface RichTextProps {
  html: string;
  className?: string;
  density?: RichTextDensity;
}

const BARE_URL_PATTERN = /https?:\/\/[^\s<>"']+/gi;
const TRAILING_URL_PUNCTUATION = /[.,;:!?，。；：！？、]+$/;
const AUTOLINK_BLOCKED_TAGS = new Set(["a", "code", "pre"]);

function linkifyText(text: string) {
  return text.replace(BARE_URL_PATTERN, (match) => {
    const trailing = match.match(TRAILING_URL_PUNCTUATION)?.[0] ?? "";
    const url = trailing ? match.slice(0, -trailing.length) : match;
    if (!url) return match;

    return `<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>${trailing}`;
  });
}

function linkifyBareUrlsInHtml(html: string) {
  let blockedDepth = 0;

  return html
    .split(/(<[^>]+>)/g)
    .map((token) => {
      if (!token.startsWith("<")) {
        return blockedDepth > 0 ? token : linkifyText(token);
      }

      const closingTag = token.match(/^<\s*\/\s*([a-z0-9-]+)/i)?.[1];
      if (closingTag && AUTOLINK_BLOCKED_TAGS.has(closingTag.toLowerCase())) {
        blockedDepth = Math.max(0, blockedDepth - 1);
        return token;
      }

      const openingTag = token.match(/^<\s*([a-z0-9-]+)/i)?.[1];
      if (
        openingTag &&
        AUTOLINK_BLOCKED_TAGS.has(openingTag.toLowerCase()) &&
        !/\/\s*>$/.test(token)
      ) {
        blockedDepth += 1;
      }
      return token;
    })
    .join("");
}

/**
 * 富文本渲染组件
 * 使用DOMPurify净化HTML，防止XSS攻击
 * 支持纯文本和HTML格式
 * 自动将图片URL转换为<img>标签
 */
function RichText({
  html,
  className = "",
  density = "reading",
}: RichTextProps) {
  // 净化HTML，移除危险的标签和属性
  const cleanHtml = useMemo(() => {
    let contentToSanitize = html;

    // 无论是否是HTML，都先处理独立的图片URL（不在标签内的URL）
    // 匹配不在HTML标签属性中的图片URL
    contentToSanitize = contentToSanitize.replace(
      /(^|[^="'>])(https?:\/\/[^\s<>]+\.(?:jpg|jpeg|png|gif|webp|svg|bmp)(?:\?[^\s<>]*)?)/gi,
      '$1<img src="$2" alt="show notes image" class="rounded-lg my-2" style="max-width: 100%; height: auto;" loading="lazy" />',
    );

    // 检查是否包含HTML标签
    const hasHtmlTags =
      /<\/?[a-z][\s\S]*?>/i.test(contentToSanitize) &&
      !/^<a\s+href[^>]*>.*<\/a>$/i.test(contentToSanitize);

    // 如果是纯文本（没有HTML标签），将换行符转换为<br>
    if (!hasHtmlTags) {
      contentToSanitize = contentToSanitize.replace(/\n/g, "<br>");
    }

    const sanitizedHtml = sanitizeRichTextHtml(contentToSanitize);
    const linkedHtml = linkifyBareUrlsInHtml(sanitizedHtml);
    return optimizeHtmlImageSources(sanitizeRichTextHtml(linkedHtml));
  }, [html]);

  return (
    <div
      className={`rich-text ${getRichTextClassName(density, className)}`}
      dangerouslySetInnerHTML={{ __html: cleanHtml }}
    />
  );
}

export default memo(RichText);

RichText.displayName = "RichText";
