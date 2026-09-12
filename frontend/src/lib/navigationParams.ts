export function singleParam(params: URLSearchParams, key: string) {
  const values = params.getAll(key);
  return values.length === 1 ? values[0] : null;
}
export function positiveID(value: string | null | undefined) {
  return value &&
    /^[1-9]\d*$/.test(value) &&
    Number.isSafeInteger(Number(value))
    ? Number(value)
    : null;
}
export function episodeIDFromHref(href: string) {
  return positiveID(/^\/episodes\/([^/?#]+)\/?(?:[?#]|$)/.exec(href)?.[1]);
}
export function episodeHref(id: number, params: Record<string, string> = {}) {
  const query = new URLSearchParams(params).toString();
  return `/episodes/${id}${query ? `?${query}` : ""}`;
}
