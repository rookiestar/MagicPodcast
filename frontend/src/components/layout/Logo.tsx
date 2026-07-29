interface LogoProps {
  className?: string;
  showText?: boolean;
  variant?: "default" | "compact" | "icon-only";
}

function Logo({
  className = "",
  showText = true,
  variant = "default",
}: LogoProps) {
  if (!showText || variant === "icon-only") {
    return (
      <span className={`magic-wordmark-mark ${className}`} aria-label="MagicPodcast">
        MP
      </span>
    );
  }

  return (
    <span className={`magic-wordmark ${className}`} aria-label="MagicPodcast">
      <strong>MAGIC</strong>
      <span>PODCAST · 01</span>
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
