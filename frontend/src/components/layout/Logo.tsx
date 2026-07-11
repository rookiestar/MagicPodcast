import React from "react";

interface LogoProps {
  size?: number;
  className?: string;
  showText?: boolean;
  variant?: "default" | "compact" | "icon-only";
}

/**
 * MagicPodcast Logo组件
 *
 * 设计理念：
 * - 音频波形 + 魔法星星，象征播客内容的魔法力量
 * - 渐变色：violet-600 到 indigo-600
 * - 简洁、现代、易识别
 */
function Logo({
  size = 40,
  className = "",
  showText = true,
  variant = "default",
}: LogoProps) {
  // 根据variant确定显示模式
  const shouldShowText = variant !== "icon-only" && showText;

  // 图标尺寸
  const iconSize = variant === "compact" ? 24 : size;

  return (
    <div className={`flex items-center gap-2 ${className}`}>
      {/* Logo Icon - 音频波形 + 魔法星星 */}
      <svg
        width={iconSize}
        height={iconSize}
        viewBox="0 0 48 48"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="flex-shrink-0"
      >
        <defs>
          <linearGradient
            id="logoGradient"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#9333ea" /> {/* violet-600 */}
            <stop offset="100%" stopColor="#4f46e5" /> {/* indigo-600 */}
          </linearGradient>
        </defs>

        {/* 音频波形 - 3条竖线 */}
        <rect
          x="10"
          y="18"
          width="6"
          height="12"
          rx="3"
          fill="url(#logoGradient)"
        />
        <rect
          x="21"
          y="12"
          width="6"
          height="24"
          rx="3"
          fill="url(#logoGradient)"
        />
        <rect
          x="32"
          y="16"
          width="6"
          height="16"
          rx="3"
          fill="url(#logoGradient)"
        />

        {/* 魔法星星 - 右上角 */}
        <path
          d="M 38 8 L 39.5 11 L 43 11 L 40 13.5 L 41 17 L 38 14.5 L 35 17 L 36 13.5 L 33 11 L 36.5 11 Z"
          fill="#fbbf24"
          opacity="0.9"
        />

        {/* 小星星装饰 */}
        <circle cx="8" cy="10" r="1.5" fill="#fbbf24" opacity="0.6" />
        <circle cx="42" cy="22" r="1" fill="#fbbf24" opacity="0.4" />
      </svg>

      {/* Logo Text */}
      {shouldShowText && (
        <div className="flex flex-col">
          <span className="text-xl font-bold bg-gradient-to-r from-violet-600 to-indigo-600 bg-clip-text text-transparent leading-tight">
            MagicPodcast
          </span>
        </div>
      )}
    </div>
  );
}

/**
 * 紧凑版Logo - 用于导航栏等空间受限的场景
 */
export function CompactLogo({
  className = "",
  size = 32,
}: {
  className?: string;
  size?: number;
}) {
  return <Logo size={size} className={className} variant="compact" />;
}
