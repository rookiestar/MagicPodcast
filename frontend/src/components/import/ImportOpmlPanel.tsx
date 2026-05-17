import type { ChangeEventHandler } from "react";

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
      <div className="mb-6 rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-900/60">
        <h3 className="mb-2 text-base font-medium text-slate-900 dark:text-slate-100">
          关于导入OPML
        </h3>
        <ul className="list-inside list-disc space-y-1 text-sm text-slate-600 dark:text-slate-300">
          <li>仅从本地PodcastIndex数据库匹配播客信息（快速）</li>
          <li>导入完成后可选择是否在线同步最新元数据</li>
          <li>支持从小宇宙、Apple Podcasts等应用导出的OPML文件</li>
        </ul>
      </div>

      <div className="mb-6">
        <label className="mb-2 block text-sm font-medium text-slate-700 dark:text-slate-300">
          选择OPML文件
        </label>
        <input
          type="file"
          accept=".opml,.xml"
          onChange={onFileChange}
          disabled={disabled}
          className="block w-full text-sm text-slate-500 file:mr-4 file:rounded-lg file:border-0 file:bg-blue-50 file:px-4 file:py-2 file:text-sm file:font-semibold file:text-blue-700 hover:file:bg-blue-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 disabled:file:bg-slate-100 disabled:file:text-slate-400 dark:text-slate-400"
        />
        {file && (
          <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">
            已选择: {file.name} ({(file.size / 1024).toFixed(2)} KB)
          </p>
        )}
      </div>

      <div className="mb-6">
        <button
          type="button"
          onClick={onImport}
          disabled={!file || disabled}
          className={`min-h-[44px] rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            !file || disabled
              ? "cursor-not-allowed bg-slate-300 dark:bg-slate-700"
              : "cursor-pointer bg-green-600 hover:bg-green-700 dark:hover:bg-green-700"
          }`}
        >
          {importing ? "导入中..." : "开始导入"}
        </button>
      </div>
    </>
  );
}
