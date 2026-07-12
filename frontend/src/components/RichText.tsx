"use client";

import { memo, useMemo } from "react";
import { sanitizeRichTextHtml } from "@/lib/contentSanitizer";
import { optimizeHtmlImageSources } from "@/lib/imageOptimization";

interface RichTextProps {
  html: string;
  className?: string;
}

/**
 * 富文本渲染组件
 * 使用DOMPurify净化HTML，防止XSS攻击
 * 支持纯文本和HTML格式
 * 自动将图片URL转换为<img>标签
 */
function RichText({ html, className = "" }: RichTextProps) {
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

    // 如果是纯文本（没有HTML标签），将换行符转换为<br>，并处理其他链接
    if (!hasHtmlTags) {
      contentToSanitize = contentToSanitize
        // 处理换行符
        .replace(/\n/g, "<br>")
        // 处理其他URL（非图片）
        .replace(
          /(https?:\/\/[^\s<>]+)(?<!\.(jpg|jpeg|png|gif|webp|svg|bmp))/gi,
          '<a href="$1" target="_blank" rel="noopener noreferrer">$1</a>',
        );
    }

    return optimizeHtmlImageSources(sanitizeRichTextHtml(contentToSanitize));
  }, [html]);

  return (
    <div
      className={`rich-text prose prose-slate max-w-none dark:prose-invert ${className}`}
      dangerouslySetInnerHTML={{ __html: cleanHtml }}
    />
  );
}

export default memo(RichText);

RichText.displayName = "RichText";
