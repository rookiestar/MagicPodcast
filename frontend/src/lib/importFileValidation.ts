const VALID_OPML_MIME_TYPES = new Set([
  "application/xml",
  "text/xml",
  "text/opml",
  "text/x-opml",
]);

const VALID_OPML_EXTENSIONS = new Set(["opml", "xml"]);

export const MAX_OPML_FILE_SIZE_BYTES = 8 * 1024 * 1024;

export function isValidOpmlFile(file: Pick<File, "name" | "type">) {
  const fileExtension = file.name.split(".").pop()?.toLowerCase() || "";

  return (
    VALID_OPML_MIME_TYPES.has(file.type) ||
    VALID_OPML_EXTENSIONS.has(fileExtension)
  );
}

export function isOpmlFileSizeAllowed(file: Pick<File, "size">) {
  return file.size <= MAX_OPML_FILE_SIZE_BYTES;
}
