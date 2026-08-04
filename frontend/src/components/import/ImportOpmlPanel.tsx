import type { ChangeEventHandler } from "react";
import { IconFileUpload } from "@tabler/icons-react";

interface ImportOpmlPanelProps {
  file: File | null;
  disabled: boolean;
  importing: boolean;
  onFileChange: ChangeEventHandler<HTMLInputElement>;
  onImport: () => void;
}

export default function ImportOpmlPanel({
  file,
  disabled,
  importing,
  onFileChange,
  onImport,
}: ImportOpmlPanelProps) {
  return (
    <>
      <div className="import-guidance">
        <p className="import-eyebrow">从其他应用迁移</p>
        <h3 className="text-base font-medium text-slate-900 dark:text-slate-100">
          导入 OPML
        </h3>
        <p className="import-guidance-copy">
          读取小宇宙、Apple Podcasts 等应用导出的订阅列表，先从本地索引快速匹配。
        </p>
      </div>

      <div className="import-file-field">
        <label
          htmlFor="opml-file-input"
          className="import-file-picker"
        >
          <IconFileUpload aria-hidden="true" />
          <span>{file ? "更换 OPML 文件" : "选择 OPML 文件"}</span>
          <input
            id="opml-file-input"
            type="file"
            accept=".opml,.xml"
            onChange={onFileChange}
            disabled={disabled}
            className="sr-only"
          />
        </label>
        <p className="import-file-name">
          {file
            ? `${file.name} · ${(file.size / 1024).toFixed(2)} KB`
            : "支持 .opml 与 .xml 文件"}
        </p>
      </div>

      <div className="import-primary-action">
        <button
          type="button"
          onClick={onImport}
          disabled={!file || disabled}
          className={`editorial-btn editorial-btn--primary min-h-[44px] px-6 py-2.5 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            !file || disabled
              ? "cursor-not-allowed is-disabled"
              : "cursor-pointer"
          }`}
        >
          {importing ? "导入中..." : "开始导入"}
        </button>
      </div>
    </>
  );
}
