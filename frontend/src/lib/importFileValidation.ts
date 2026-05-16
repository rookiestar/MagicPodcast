const VALID_OPML_MIME_TYPES = new Set([
  "application/xml",
  "text/xml",
  "text/opml",
  "text/x-opml",
]);

const VALID_OPML_EXTENSIONS = new Set(["opml", "xml"]);

export function isValidOpmlFile(file: Pick<File, "name" | "type">) {
  const fileExtension = file.name.split(".").pop()?.toLowerCase() || "";

  return (
    VALID_OPML_MIME_TYPES.has(file.type) ||
    VALID_OPML_EXTENSIONS.has(fileExtension)
  );
}
