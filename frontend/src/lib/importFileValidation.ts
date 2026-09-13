const VALID_OPML_EXTENSIONS = new Set(["opml", "xml"]);

export const MAX_OPML_FILE_SIZE_BYTES = 8 * 1024 * 1024;

// 与服务端保持同一判定：扩展名忽略大小写且必须是 .opml/.xml。MIME 只作
// 辅助展示，不参与拦截——空 MIME、伪装 MIME 都不改变结果，保证前后端
// 对同一文件给出一致结论（#398 R11）。
export function isValidOpmlFile(file: Pick<File, "name" | "type">) {
  const fileExtension = file.name.split(".").pop()?.toLowerCase() || "";
  return VALID_OPML_EXTENSIONS.has(fileExtension);
}

export function isOpmlFileSizeAllowed(file: Pick<File, "size">) {
  return file.size <= MAX_OPML_FILE_SIZE_BYTES;
}
