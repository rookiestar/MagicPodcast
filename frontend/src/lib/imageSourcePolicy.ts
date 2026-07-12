// These are the image hosts observed in the current podcast/episode data and
// reviewed as part of the single-owner deployment. New hosts must be added
// deliberately in both the frontend policy and the backend proxy allowlist.
export const APPROVED_IMAGE_HOSTS = [
  "assets.fireside.fm",
  "assets.pippa.io",
  "a1.mzstatic.com",
  "a2.mzstatic.com",
  "a3.mzstatic.com",
  "a4.mzstatic.com",
  "a5.mzstatic.com",
  "bts-image.xyzcdn.net",
  "cdn.justinbot.com",
  "cdn.lizhi.fm",
  "cdn.vistopia.com.cn",
  "cdn.wavpub.com",
  "cdn.wlz.danlirencomedy.com",
  "cdn2.jjldbk.com",
  "cdn2.wavpub.com",
  "cdn5.vistopia.com.cn",
  "content.production.cdn.art19.com",
  "crazy.capital",
  "d3t3ozftmdmh3i.cloudfront.net",
  "face.t.sinajs.cn",
  "fdfs.xmcdn.com",
  "files.fireside.fm",
  "host.podapi.xyz",
  "hosting.wavpub.cn",
  "i.typlog.com",
  "image-qiniu.jellow.site",
  "image.firstory-cdn.me",
  "image.xyzcdn.net",
  "images.pexels.com",
  "images.unsplash.com",
  "imagev2.xmcdn.com",
  "img.transistorcdn.com",
  "is1-ssl.mzstatic.com",
  "is2-ssl.mzstatic.com",
  "is3-ssl.mzstatic.com",
  "is4-ssl.mzstatic.com",
  "is5-ssl.mzstatic.com",
  "jsftwafp1d.feishu.cn",
  "justpodmedia.com",
  "lexfridman.com",
  "media.redcircle.com",
  "media.smfm2016.com",
  "media.wavpub.com",
  "media24.fireside.fm",
  "megaphone.imgix.net",
  "mmbiz.qpic.cn",
  "pan.icu",
  "pie.wetime.com",
  "radio-res.cgtn.com",
  "rio.xyzcdn.net",
  "s.anyway.red",
  "s.w.org",
  "static.storyfm.cn",
  "static2.ximalaya.com",
  "storage.buzzsprout.com",
  "typlog.com",
  "uploader.shimo.im",
  "v2km9a2fuc.feishu.cn",
  "xueqiu.feishu.cn",
] as const;

const approvedImageHostSet = new Set<string>(APPROVED_IMAGE_HOSTS);

export const MAX_INLINE_IMAGE_DATA_LENGTH = 1024 * 1024;

function normalizeHostname(hostname: string) {
  return hostname.trim().toLowerCase().replace(/\.$/, "");
}

export function isApprovedImageHost(hostname: string) {
  return approvedImageHostSet.has(normalizeHostname(hostname));
}

export function isApprovedRemoteImageUrl(value: string) {
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      !parsed.username &&
      !parsed.password &&
      !parsed.port &&
      isApprovedImageHost(parsed.hostname)
    );
  } catch {
    return false;
  }
}

export function isSafeInlineImageData(value: string) {
  if (value.length > MAX_INLINE_IMAGE_DATA_LENGTH) {
    return false;
  }

  return /^data:image\/png;base64,[a-z0-9+/=]+$/i.test(value);
}

/**
 * Return a same-origin image URL for content rendering. Approved remote
 * images always go through the bounded backend proxy; unapproved sources are
 * omitted instead of being loaded directly by the browser or Next optimizer.
 */
export function getSafeImageSource(
  value: string | undefined | null,
): string | undefined {
  const source = value?.trim();
  if (!source) {
    return undefined;
  }

  if (isSafeInlineImageData(source)) {
    return source;
  }

  if (source.startsWith("/") && !source.startsWith("//")) {
    return source;
  }

  // Bare relative paths resolve against the same application origin and do
  // not create an external request. URL-like strings with a scheme are
  // handled by the reviewed-host check below.
  if (!source.includes(":") && !source.startsWith("//")) {
    return source;
  }

  if (!isApprovedRemoteImageUrl(source)) {
    return undefined;
  }

  const proxyPath = `/images/proxy?url=${encodeURIComponent(source)}`;
  return proxyPath;
}

export function sanitizeContentUrl(value: string | undefined | null) {
  const source = value?.trim();
  if (!source || source.startsWith("//")) {
    return "";
  }

  if (
    /^qr-placeholder-\d+$/.test(source) ||
    source.startsWith("/") ||
    source.startsWith("#") ||
    /^https?:\/\//i.test(source) ||
    /^(?:mailto|tel):/i.test(source)
  ) {
    return source;
  }

  return "";
}
