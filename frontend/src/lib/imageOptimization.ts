const DEFAULT_IMAGE_WIDTH = 128;
const DEFAULT_IMAGE_QUALITY = 75;
const IMAGE_OPTIMIZER_PATH =
  process.env.NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH || "/_next/image";

export function isOptimizableImageUrl(src: string) {
  if (!src) {
    return false;
  }

  if (src.startsWith(IMAGE_OPTIMIZER_PATH)) {
    return false;
  }

  if (src.startsWith("data:") || src.startsWith("blob:")) {
    return false;
  }

  return (
    src.startsWith("http://") ||
    src.startsWith("https://") ||
    src.startsWith("/")
  );
}

export function getOptimizedImageUrl(
  src: string,
  width = DEFAULT_IMAGE_WIDTH,
  quality = DEFAULT_IMAGE_QUALITY,
) {
  if (!isOptimizableImageUrl(src)) {
    return src;
  }

  const queryParams = new URLSearchParams({
    url: src,
    w: width.toString(),
    q: quality.toString(),
  });

  return `${IMAGE_OPTIMIZER_PATH}?${queryParams.toString()}`;
}

export function optimizeHtmlImageSources(
  html: string,
  width = 768,
  quality = DEFAULT_IMAGE_QUALITY,
) {
  return html.replace(/\bsrc=(["'])([^"']+)\1/gi, (match, quote, src) => {
    if (!isOptimizableImageUrl(src)) {
      return match;
    }

    return `src=${quote}${getOptimizedImageUrl(src, width, quality)}${quote}`;
  });
}

export function canUseNextImage(src: string) {
  return (
    src.startsWith("/") ||
    src.startsWith("http://") ||
    src.startsWith("https://") ||
    src.startsWith("data:")
  );
}
