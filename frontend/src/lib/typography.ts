export type RichTextDensity = "reading" | "compact";

export function getRichTextClassName(
  density: RichTextDensity,
  className = "",
): string {
  return [
    "editorial-rich-text",
    `editorial-rich-text--${density}`,
    className.trim(),
  ]
    .filter(Boolean)
    .join(" ");
}
