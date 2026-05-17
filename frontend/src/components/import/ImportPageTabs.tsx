export type ImportTab = "import" | "sync";

interface ImportPageTabsProps {
  activeTab: ImportTab;
  disabled: boolean;
  onChange: (tab: ImportTab) => void;
}

function tabClassName(active: boolean, disabled: boolean) {
  return `min-h-[44px] rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
    active
      ? "bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-blue-300"
      : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
  } ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`;
}

export default function ImportPageTabs({
  activeTab,
  disabled,
  onChange,
}: ImportPageTabsProps) {
  return (
    <div className="border-b border-slate-200 px-6 py-5 dark:border-slate-700">
      <div className="inline-flex rounded-lg border border-slate-200 bg-slate-100 p-1 dark:border-slate-700 dark:bg-slate-900">
        <button
          type="button"
          onClick={() => onChange("import")}
          disabled={disabled}
          aria-pressed={activeTab === "import"}
          className={tabClassName(activeTab === "import", disabled)}
        >
          导入 OPML
        </button>
        <button
          type="button"
          onClick={() => onChange("sync")}
          disabled={disabled}
          aria-pressed={activeTab === "sync"}
          className={tabClassName(activeTab === "sync", disabled)}
        >
          同步元数据
        </button>
      </div>
    </div>
  );
}
