"use client";

import { useState, memo, useMemo } from "react";
import Image from "next/image";
import { getProxiedImageUrl } from "@/lib/imageProxy";

interface PodcastCoverProps {
  coverUrl?: string;
  title: string;
  index?: number;
  priority?: "high" | "medium" | "low";
}

function PodcastCover({
  coverUrl,
  title,
  index = 0,
  priority = "medium",
}: PodcastCoverProps) {
  const [imageError, setImageError] = useState(false);

  // 获取图片URL（优先使用代理URL）
  const imageUrl = coverUrl ? getProxiedImageUrl(coverUrl) || coverUrl : "";

  // 检查是否是代理URL（代理URL使用普通img标签，避免Next.js图片优化器的HEAD请求问题）
  // 匹配 /images/proxy 或 http://localhost:8080/images/proxy 等格式
  const isProxiedUrl = imageUrl.includes("/images/proxy");

  // 根据优先级设置Next.js Image的priority属性
  const isHighPriority = priority === "high" || index < 6;

  // 如果没有封面URL或加载失败，显示占位符
  if (!imageUrl || imageError) {
    return (
      <div className="aspect-square bg-slate-200 w-full h-full flex items-center justify-center">
        <div className="text-5xl text-slate-400">🎧</div>
      </div>
    );
  }

  // 对于代理URL，使用普通img标签（绕过Next.js图片优化器的HEAD请求问题）
  if (isProxiedUrl) {
    return (
      <div className="aspect-square bg-slate-200 relative w-full h-full overflow-hidden">
        <img
          src={imageUrl}
          alt={title}
          className="object-cover w-full h-full"
          loading={isHighPriority ? "eager" : "lazy"}
          onError={() => setImageError(true)}
        />
      </div>
    );
  }

  return (
    <div className="aspect-square bg-slate-200 relative w-full h-full overflow-hidden">
      {/* 使用Next.js Image组件 */}
      <Image
        src={imageUrl}
        alt={title}
        fill
        sizes="(max-width: 640px) 50vw, (max-width: 828px) 33vw, (max-width: 1200px) 20vw, 256px"
        className="object-cover"
        priority={isHighPriority}
        loading={isHighPriority ? "eager" : "lazy"}
        onError={() => setImageError(true)}
      />
    </div>
  );
}

// 自定义比较函数：只在关键 props 变化时才重新渲染
function arePropsEqual(
  prevProps: Readonly<PodcastCoverProps>,
  nextProps: Readonly<PodcastCoverProps>,
) {
  return (
    prevProps.coverUrl === nextProps.coverUrl &&
    prevProps.title === nextProps.title &&
    prevProps.index === nextProps.index &&
    prevProps.priority === nextProps.priority
  );
}

// 使用 React.memo 包装组件
export default memo(PodcastCover, arePropsEqual);

// 添加 displayName 用于调试
PodcastCover.displayName = "PodcastCover";
