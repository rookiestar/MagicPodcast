import { IconRefresh } from "@tabler/icons-react";

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
      <div className="import-guidance">
        <p className="import-eyebrow">更新个人播客库</p>
        <h3 className="text-base font-medium text-slate-900 dark:text-slate-100">
          同步元数据
        </h3>
        <p className="import-guidance-copy">
          从 RSS 更新单集数量、发布时间与节目描述。耗时取决于订阅数量和网络状况。
        </p>
      </div>

      <div className="import-primary-action">
        <button
          type="button"
          onClick={onSync}
          disabled={disabled}
          className={`editorial-btn editorial-btn--primary min-h-[44px] px-6 py-2.5 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
            disabled
              ? "cursor-not-allowed is-disabled"
              : "cursor-pointer"
          }`}
        >
          <IconRefresh aria-hidden="true" />
          {syncing ? "同步中..." : "开始同步"}
        </button>
      </div>
    </>
  );
}
