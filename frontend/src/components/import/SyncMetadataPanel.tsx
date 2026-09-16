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
          同步已关注节目
        </h3>
        <p className="import-guidance-copy">
          检查全部已关注节目的 RSS：更新节目资料（单集数量、发布时间与描述），
          并按各节目的同步范围新增或更新单集。耗时取决于已关注节目数量和网络状况。
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
