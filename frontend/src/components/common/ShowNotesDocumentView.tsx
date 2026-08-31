"use client";

import type { ReactNode } from "react";
import RichText from "@/components/RichText";
import MarkdownViewer from "@/components/workflows/MarkdownViewer";
import type { RichTextDensity } from "@/lib/typography";
import type { ShowNotesDocument } from "@/types/showNotes";

interface ShowNotesDocumentViewProps {
  document: ShowNotesDocument;
  className?: string;
  density?: RichTextDensity;
  emptyFallback?: ReactNode;
}

export function ShowNotesDocumentView({
  document,
  className,
  density = "reading",
  emptyFallback = null,
}: ShowNotesDocumentViewProps) {
  if (!document.content.trim()) return <>{emptyFallback}</>;

  return document.format === "html" ? (
    <RichText html={document.content} className={className} density={density} />
  ) : (
    <MarkdownViewer
      content={document.content}
      className={className}
      density={density}
    />
  );
}
