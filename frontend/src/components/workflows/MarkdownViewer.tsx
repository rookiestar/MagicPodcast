"use client";

import React, { useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeRaw from "rehype-raw";
import PlainImage from "@/components/ui/PlainImage";
import { getOptimizedImageUrl } from "@/lib/imageOptimization";

interface MarkdownViewerProps {
  content: string;
  className?: string;
}

export default function MarkdownViewer({
  content,
  className = "",
}: MarkdownViewerProps) {
  // 预处理：提取所有二维码的base64数据并替换为占位符
  const { qrCodesList, cleanedContent } = useMemo(() => {
    const list: string[] = [];
    const regex = /!\[二维码\]\(data:image\/png;base64,([a-zA-Z0-9+/=]+)\)/g;

    // 替换所有二维码语法为占位符
    const cleaned = content.replace(regex, (_, base64) => {
      list.push(base64);
      return `![二维码](qr-placeholder-${list.length - 1})`;
    });

    return { qrCodesList: list, cleanedContent: cleaned };
  }, [content]);

  return (
    <div
      className={`prose prose-slate dark:prose-invert max-w-none ${className}`}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={{
          img: ({ src, alt, ...props }) => {
            // 处理二维码占位符
            if (alt === "二维码" && src?.includes("qr-placeholder-")) {
              const match = src.match(/qr-placeholder-(\d+)/);
              if (match) {
                const index = parseInt(match[1], 10);
                const base64 = qrCodesList[index];

                if (base64) {
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
            return (
              <PlainImage
                src={getOptimizedImageUrl(imageSrc, 768, 80)}
                alt={alt || ""}
                loading="lazy"
                decoding="async"
                className="my-3 max-w-full rounded-lg"
                {...props}
              />
            );
          },
          h1: ({ children }) => (
            <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-50 border-b border-slate-200 dark:border-slate-700 pb-2 mb-4">
              {children}
            </h1>
          ),
          h2: ({ children }) => (
            <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mt-6 mb-3">
              {children}
            </h2>
          ),
          h3: ({ children }) => (
            <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50 mt-4 mb-2">
              {children}
            </h3>
          ),
          h4: ({ children }) => (
            <h4 className="text-base font-semibold text-slate-900 dark:text-slate-50 mt-3 mb-2">
              {children}
            </h4>
          ),
          p: ({ children }) => (
            <p className="text-slate-700 dark:text-slate-300 leading-relaxed mb-3">
              {children}
            </p>
          ),
          ul: ({ children }) => (
            <ul className="list-disc list-inside text-slate-700 dark:text-slate-300 mb-3 space-y-1">
              {children}
            </ul>
          ),
          ol: ({ children }) => (
            <ol className="list-decimal list-inside text-slate-700 dark:text-slate-300 mb-3 space-y-1">
              {children}
            </ol>
          ),
          a: ({ href, children }) => (
            <a
              href={href as string}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300 underline"
            >
              {children}
            </a>
          ),
          hr: () => (
            <hr className="my-4 border-slate-300 dark:border-slate-700" />
          ),
          strong: ({ children }) => (
            <strong className="font-semibold text-slate-900 dark:text-slate-50">
              {children}
            </strong>
          ),
          code: ({ children }) => (
            <code className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded text-sm font-mono text-slate-900 dark:text-slate-50">
              {children}
            </code>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-slate-300 dark:border-slate-700 pl-4 italic text-slate-600 dark:text-slate-400 my-4">
              {children}
            </blockquote>
          ),
        }}
      >
        {cleanedContent}
      </ReactMarkdown>
    </div>
  );
}
