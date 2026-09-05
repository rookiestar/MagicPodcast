"use client";

import React, { useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import PlainImage from "@/components/ui/PlainImage";
import {
  getSafeImageSource,
  isSafeInlineImageData,
  sanitizeContentUrl,
} from "@/lib/imageSourcePolicy";
import { sanitizeMarkdownSource } from "@/lib/contentSanitizer";
import {
  getOptimizedImageUrl,
  RICH_TEXT_IMAGE_WIDTH,
} from "@/lib/imageOptimization";
import {
  getRichTextClassName,
  type RichTextDensity,
} from "@/lib/typography";

interface MarkdownViewerProps {
  content: string;
  className?: string;
  density?: RichTextDensity;
  renderImage?: (image: {
    src: string;
    alt: string;
  }) => React.ReactNode | undefined;
}

export default function MarkdownViewer({
  content,
  className = "",
  density = "reading",
  renderImage,
}: MarkdownViewerProps) {
  // 预处理：提取所有二维码的base64数据并替换为占位符
  const { qrCodesList, cleanedContent } = useMemo(() => {
    const list: string[] = [];
    const regex = /!\[二维码\]\(data:image\/png;base64,([a-zA-Z0-9+/=]+)\)/g;

    // 替换所有二维码语法为占位符
    const cleaned = content.replace(regex, (_, base64) => {
      const inlineData = `data:image/png;base64,${base64}`;
      if (!isSafeInlineImageData(inlineData)) {
        return "";
      }
      list.push(base64);
      return `![二维码](qr-placeholder-${list.length - 1})`;
    });

    return {
      qrCodesList: list,
      cleanedContent: sanitizeMarkdownSource(cleaned),
    };
  }, [content]);

  return (
    <div className={getRichTextClassName(density, className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={(url) => sanitizeContentUrl(url)}
        components={{
          img: ({ src, alt, ...props }) => {
            // 处理二维码占位符
            if (alt === "二维码" && src?.includes("qr-placeholder-")) {
              const match = src.match(/qr-placeholder-(\d+)/);
              if (match) {
                const index = parseInt(match[1], 10);
                const base64 = qrCodesList[index];

                if (base64 && isSafeInlineImageData(`data:image/png;base64,${base64}`)) {
                  return (
                    <PlainImage
                      src={`data:image/png;base64,${base64}`}
                      alt="二维码"
                      width="128"
                      height="128"
                      loading="lazy"
                      decoding="async"
                      className="inline-block mx-2"
                      style={{
                        cursor: "pointer",
                        display: "inline-block",
                        margin: "4px",
                      }}
                    />
                  );
                }
              }
            }
            // 其他图片正常渲染
            const imageSrc = typeof src === "string" ? src : "";
            if (!getSafeImageSource(imageSrc)) {
              return null;
            }
            const customImage = renderImage?.({
              src: imageSrc,
              alt: alt || "",
            });
            if (customImage !== undefined) {
              return customImage;
            }
            const safeImageSrc = getOptimizedImageUrl(
              imageSrc,
              RICH_TEXT_IMAGE_WIDTH,
            );
            if (!safeImageSrc) {
              return null;
            }
            return (
              <PlainImage
                src={safeImageSrc}
                alt={alt || ""}
                loading="lazy"
                decoding="async"
                className="my-3 max-w-full rounded-lg"
                {...props}
              />
            );
          },
          a: ({ href, children }) => {
            const safeHref = sanitizeContentUrl(href);
            if (!safeHref) {
              return <span>{children}</span>;
            }
            return (
              <a
                href={safeHref}
                target="_blank"
                rel="noopener noreferrer"
              >
                {children}
              </a>
            );
          },
        }}
      >
        {cleanedContent}
      </ReactMarkdown>
    </div>
  );
}
