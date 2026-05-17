const DEFAULT_IMAGE_WIDTH = 128;
const DEFAULT_IMAGE_QUALITY = 75;
const IMAGE_OPTIMIZER_PATH =
  process.env.NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH || "/_next/image";

export function getOptimizedImageUrl(
  src: string,
  width = DEFAULT_IMAGE_WIDTH,
  quality = DEFAULT_IMAGE_QUALITY,
) {
  if (!src) {
    return "";
  }

  if (src.startsWith("data:") || src.startsWith("blob:")) {
    return src;
  }

  if (
    !src.startsWith("http://") &&
    !src.startsWith("https://") &&
    !src.startsWith("/")
  ) {
    return src;
  }

  const queryParams = new URLSearchParams({
    url: src,
    w: width.toString(),
    q: quality.toString(),
  });

  return `${IMAGE_OPTIMIZER_PATH}?${queryParams.toString()}`;
}
