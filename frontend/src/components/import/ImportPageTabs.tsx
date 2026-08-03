export type ImportTab = "import" | "sync";

const IMPORT_TABS: ReadonlyArray<{ key: ImportTab; label: string }> = [
  { key: "import", label: "导入 OPML" },
  { key: "sync", label: "同步元数据" },
];

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
  // 键盘导航：方向键/Home/End 在 tab 间移动焦点（roving tabindex）。
  const onKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>, idx: number) => {
    const last = IMPORT_TABS.length - 1;
    let next = idx;
    if (e.key === "ArrowRight" || e.key === "ArrowDown") next = idx >= last ? 0 : idx + 1;
    else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = idx <= 0 ? last : idx - 1;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = last;
    else return;
    e.preventDefault();
    onChange(IMPORT_TABS[next].key);
    document.getElementById(`import-tab-${IMPORT_TABS[next].key}`)?.focus();
  };

  return (
    <div className="border-b border-slate-200 px-6 py-5 dark:border-slate-700">
      <div
        className="inline-flex rounded-lg border border-slate-200 bg-slate-100 p-1 dark:border-slate-700 dark:bg-slate-900"
        role="tablist"
        aria-label="导入方式"
      >
        {IMPORT_TABS.map((tab, idx) => {
          const active = activeTab === tab.key;
          return (
            <button
              key={tab.key}
              id={`import-tab-${tab.key}`}
              type="button"
              role="tab"
              aria-selected={active}
              aria-controls={`import-tabpanel-${tab.key}`}
              tabIndex={active ? 0 : -1}
              onClick={() => onChange(tab.key)}
              onKeyDown={(e) => onKeyDown(e, idx)}
              disabled={disabled}
              className={tabClassName(active, disabled)}
            >
              {tab.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
