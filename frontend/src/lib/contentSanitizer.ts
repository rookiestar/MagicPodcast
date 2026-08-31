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
    "hr",
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

const RICH_TEXT_TAGS = new Set(RICH_TEXT_OPTIONS.ALLOWED_TAGS);
const RICH_TEXT_ATTRS = new Set(RICH_TEXT_OPTIONS.ALLOWED_ATTR);
const SANITIZER_ROOT_GUARD = "__MAGICPODCAST_RICH_TEXT_ROOT__";

function isAllowedContentUrl(value: string) {
  return RICH_TEXT_OPTIONS.ALLOWED_URI_REGEXP.test(value.trim());
}

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

function stripUnsafeRawHtmlAttributes(value: string) {
  return value
    .replace(/\s+on[a-z][\w:-]*\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(/\s+style\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "")
    .replace(/\s+srcset\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)/gi, "");
}

function sanitizeRichTextFallback(html: string) {
  if (typeof document === "undefined") {
    return stripUnsafeRawHtmlAttributes(stripDangerousTags(html));
  }

  const container = document.createElement("div");
  container.innerHTML = stripDangerousTags(html);

  const walk = (parent: Element) => {
    for (const child of Array.from(parent.childNodes)) {
      if (child.nodeType !== 1) {
        continue;
      }

      const element = child as Element;
      const tagName = element.tagName.toLowerCase();
      if (COMMON_FORBIDDEN_TAGS.includes(tagName)) {
        element.remove();
        continue;
      }

      if (!RICH_TEXT_TAGS.has(tagName)) {
        while (element.firstChild) {
          parent.insertBefore(element.firstChild, element);
        }
        element.remove();
        continue;
      }

      for (const attribute of Array.from(element.attributes)) {
        const name = attribute.name.toLowerCase();
        const keep = RICH_TEXT_ATTRS.has(name);
        const urlAttribute = name === "href" || name === "src";
        if (!keep || (urlAttribute && !isAllowedContentUrl(attribute.value))) {
          element.removeAttribute(attribute.name);
        }
      }

      walk(element);
    }
  };

  walk(container);
  return container.innerHTML;
}

function needsRichTextFallback(source: string, sanitized: string) {
  if (
    /<\s*(?:script|iframe|object|embed|svg|math|style|link|meta|form|video|audio|template)\b/i.test(
      sanitized,
    ) ||
    /\s(?:on[a-z][\w:-]*|style|srcset)\s*=/i.test(sanitized)
  ) {
    return true;
  }

  const allowedTagsInSource = source.match(
    /<\s*(p|br|hr|span|strong|b|em|i|u|a|ul|ol|li|h[1-6]|blockquote|code|pre|div|img)\b/gi,
  );
  return Boolean(
    allowedTagsInSource?.some((tag) => {
      const normalized = tag.replace(/\s|</g, "").toLowerCase();
      return !new RegExp(`<\\/?${normalized}\\b`, "i").test(sanitized);
    }),
  );
}

function stripUnsafeAnchorHrefs(html: string) {
  return html.replace(/<a\b[^>]*>/gi, (tag) => {
    const href = tag.match(/\shref\s*=\s*(["'])(.*?)\1/i)?.[2];
    if (!href || isAllowedContentUrl(href)) {
      return tag;
    }
    return tag
      .replace(/\s+href\s*=\s*(?:"[^"]*"|'[^']*')/i, "")
      .replace(/\s+target\s*=\s*(?:"[^"]*"|'[^']*')/i, "")
      .replace(/\s+rel\s*=\s*(?:"[^"]*"|'[^']*')/i, "");
  });
}

function sanitizeWithRootGuard(html: string) {
  const guardedHtml = `${SANITIZER_ROOT_GUARD}${html}`;
  const sanitized = DOMPurify.sanitize(guardedHtml, RICH_TEXT_OPTIONS);
  return sanitized.startsWith(SANITIZER_ROOT_GUARD)
    ? sanitized.slice(SANITIZER_ROOT_GUARD.length)
    : sanitized.replace(SANITIZER_ROOT_GUARD, "");
}

export function sanitizeRichTextHtml(html: string) {
  const source = stripUnsafeAnchorHrefs(stripDangerousTags(html));
  const sanitized = sanitizeWithRootGuard(source);
  const result = needsRichTextFallback(source, sanitized)
    ? sanitizeRichTextFallback(source)
    : sanitized;
  return stripUnsafeAnchorHrefs(result);
}

/**
 * Markdown is parsed by react-markdown without rehypeRaw. Removing raw HTML
 * here gives the same forbidden-tag and event-attribute boundary as RichText
 * while preserving normal Markdown syntax for headings, lists and links.
 */
export function sanitizeMarkdownSource(markdown: string) {
  // ReactMarkdown is intentionally used without rehypeRaw, so raw HTML is
  // rendered as text. Preserve Markdown syntax here and remove only the
  // dangerous raw-tag/attribute forms before parsing.
  return stripUnsafeRawHtmlAttributes(stripDangerousTags(markdown));
}
