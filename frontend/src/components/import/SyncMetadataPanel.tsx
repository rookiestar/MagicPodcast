interface SyncMetadataPanelProps {
  disabled: boolean;
  syncing: boolean;
  onSync: () => void;
}

export default function SyncMetadataPanel({
  disabled,
  syncing,
  onSync,
}: SyncMetadataPanelProps) {
  return (
    <>
      <div className="mb-6 rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-700 dark:bg-slate-900/60">
        <h3 className="mb-2 text-base font-medium text-slate-900 dark:text-slate-100">
          关于同步元数据
        </h3>
        <ul className="list-inside list-disc space-y-1 text-sm text-slate-600 dark:text-slate-300">
          <li>从在线RSS feed更新所有播客的最新元数据</li>
          <li>包括单集数量、最新发布时间、播客描述等信息</li>
          <li>可能需要较长时间,取决于播客数量和网络状况</li>
        </ul>
      </div>

      <div className="mb-6">
        <button
          type="button"
          onClick={onSync}
          disabled={disabled}
          className={`min-h-[44px] rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            disabled
              ? "cursor-not-allowed bg-slate-300 dark:bg-slate-700"
              : "cursor-pointer bg-blue-600 hover:bg-blue-700 dark:hover:bg-blue-700"
          }`}
        >
          {syncing ? "同步中..." : "开始同步"}
        </button>
      </div>
    </>
  );
}
