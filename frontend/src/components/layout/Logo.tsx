import Image from "next/image";

interface LogoProps {
  className?: string;
  showText?: boolean;
  variant?: "default" | "compact" | "icon-only";
}

export function Logo({
  className = "",
  showText = true,
  variant = "default",
}: LogoProps) {
  const iconOnly = !showText || variant === "icon-only";

  return (
    <span
      className={`magic-wordmark ${iconOnly ? "is-icon-only" : ""} ${className}`}
      role="img"
      aria-label="MagicPodcast"
      data-variant={variant}
    >
      <Image
        className="magic-wordmark-mark"
        src="/brand/magicpodcast-tuning-mark.png"
        width={32}
        height={32}
        alt=""
        priority
      />
      {!iconOnly && (
        <span className="magic-wordmark-name" aria-hidden="true">
          MagicPodcast
        </span>
      )}
    </span>
  );
}

export function CompactLogo({
  className = "",
}: {
  className?: string;
  size?: number;
}) {
  return <Logo className={className} variant="compact" />;
}
