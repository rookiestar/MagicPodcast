import { getSafeImageSource } from "./imageSourcePolicy";

export const OPTIMIZED_IMAGE_WIDTHS = [
  96,
  128,
  256,
  384,
  512,
  640,
  750,
  828,
  1080,
  1200,
  1920,
] as const;
export type OptimizedImageWidth = (typeof OPTIMIZED_IMAGE_WIDTHS)[number];

export const DEFAULT_IMAGE_WIDTH: OptimizedImageWidth = 128;
export const RICH_TEXT_IMAGE_WIDTH: OptimizedImageWidth = 750;
export const DEFAULT_IMAGE_QUALITY = 75;
const IMAGE_OPTIMIZER_PATH =
  process.env.NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH || "/_next/image.webp";

export function isOptimizableImageUrl(src: string) {
  if (!src) {
    return false;
  }

  if (src.startsWith(IMAGE_OPTIMIZER_PATH)) {
    return false;
  }

  if (
    src.startsWith("data:") ||
    src.startsWith("blob:")
  ) {
    return false;
  }

  return src.startsWith("/");
}

export function getOptimizedImageUrl(
  src: string,
  width: OptimizedImageWidth = DEFAULT_IMAGE_WIDTH,
) {
  const safeSource = getSafeImageSource(src);
  if (!safeSource) {
    return "";
  }

  if (!isOptimizableImageUrl(safeSource)) {
    return safeSource;
  }

  const queryParams = new URLSearchParams({
    url: safeSource,
    w: width.toString(),
    q: DEFAULT_IMAGE_QUALITY.toString(),
  });

  return `${IMAGE_OPTIMIZER_PATH}?${queryParams.toString()}`;
}

export function optimizeHtmlImageSources(
  html: string,
  width: OptimizedImageWidth = RICH_TEXT_IMAGE_WIDTH,
) {
  return html.replace(/\bsrc=(["'])([^"']+)\1/gi, (match, quote, src) => {
    const safeSource = getSafeImageSource(src);
    if (!safeSource) {
      return "";
    }

    if (!isOptimizableImageUrl(safeSource)) {
      return `src=${quote}${safeSource}${quote}`;
    }

    return `src=${quote}${getOptimizedImageUrl(safeSource, width)}${quote}`;
  });
}

export function canUseNextImage(src: string) {
  return (
    src.startsWith("/") ||
    src.startsWith("data:")
  );
}
