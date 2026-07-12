import DOMPurify from "dompurify";

const COMMON_FORBIDDEN_TAGS = [
  "audio",
  "embed",
  "form",
  "iframe",
  "link",
  "math",
  "meta",
  "object",
  "script",
  "style",
  "svg",
  "template",
  "video",
];

const RICH_TEXT_OPTIONS = {
  ALLOWED_TAGS: [
    "p",
    "br",
    "span",
    "strong",
    "b",
    "em",
    "i",
    "u",
    "a",
    "ul",
    "ol",
    "li",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "blockquote",
    "code",
    "pre",
    "div",
    "img",
  ],
  ALLOWED_ATTR: [
    "href",
    "title",
    "alt",
    "class",
    "target",
    "rel",
    "src",
    "width",
    "height",
    "loading",
    "decoding",
  ],
  ALLOW_DATA_ATTR: false,
  FORBID_TAGS: COMMON_FORBIDDEN_TAGS,
  FORBID_ATTR: ["style", "srcset"],
  ALLOWED_URI_REGEXP:
    /^(?:(?:https?|mailto|tel):|\/(?!\/)|#|[a-z0-9._~-]+(?:\/|$))/i,
};

function stripDangerousTags(value: string) {
  const tagNames = COMMON_FORBIDDEN_TAGS.join("|");
  return value
    .replace(
      new RegExp(
        `<\\s*(?:${tagNames})\\b[^>]*>[\\s\\S]*?<\\s*\\/\\s*(?:${tagNames})\\s*>`,
        "gi",
      ),
      "",
    )
    .replace(new RegExp(`<\\s*(?:${tagNames})\\b[^>]*\\/?>`, "gi"), "");
}

export function sanitizeRichTextHtml(html: string) {
  return DOMPurify.sanitize(stripDangerousTags(html), RICH_TEXT_OPTIONS);
}

/**
 * Markdown is parsed by react-markdown without rehypeRaw. Removing raw HTML
 * here gives the same forbidden-tag and event-attribute boundary as RichText
 * while preserving normal Markdown syntax for headings, lists and links.
 */
export function sanitizeMarkdownSource(markdown: string) {
  return DOMPurify.sanitize(stripDangerousTags(markdown), {
    ALLOWED_TAGS: [],
    ALLOWED_ATTR: [],
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: COMMON_FORBIDDEN_TAGS,
    FORBID_ATTR: ["style", "srcset"],
  });
}
