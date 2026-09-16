"use client";

import { useEffect, useMemo } from "react";
import { singleParam, updateQuery, useLocationHref } from "@/lib/navigation";
import PageLayout from "@/components/layout/PageLayout";
import ImportOpmlPanel from "@/components/import/ImportOpmlPanel";
import ImportPageTabs, { type ImportTab } from "@/components/import/ImportPageTabs";
import SyncLogPanel from "@/components/import/SyncLogPanel";
import SyncMetadataPanel from "@/components/import/SyncMetadataPanel";
import { useImportSyncOperations } from "@/hooks/useImportSyncOperations";
import { useStableLogScroll } from "@/hooks/useStableLogScroll";
import { useSyncLogSession } from "@/hooks/useSyncLogSession";

function ImportPageContent({ initialTab }: { initialTab: ImportTab }) {
  const href = useLocationHref();
  const query = useMemo(() => new URL(href || `/import?tab=${initialTab}`, "http://navigation.local").searchParams, [href, initialTab]);
  const activeTab: ImportTab = singleParam(query, "tab") === "sync" ? "sync" : "import";
  const setActiveTab = (tab: ImportTab) => updateQuery({ tab });
  useEffect(() => {
    if (query.has("tab") && !["import", "sync"].includes(singleParam(query, "tab") ?? "")) {
      updateQuery({ tab: null }, true);
    }
  }, [query]);

  const {
    logs,
    logMode,
    filter,
    setFilter,
    stats,
    filteredLogs,
    addLog,
    startLogSession,
    clearLogSession,
  } = useSyncLogSession();
  const logPanelMode = logs.length > 0 ? logMode : activeTab;
  const {
    autoScroll,
    logContainerRef,
    logEndRef,
    handleLogScroll,
    resetLogScroll,
    resumeAutoScroll,
  } = useStableLogScroll(filteredLogs.length);
  const {
    file,
    importing,
    syncing,
    preview,
    previewLoading,
    previewError,
    previewConsumed,
    canSubmitImport,
    confirmedUrls,
    lastTask,
    taskEntries,
    latestTaskError,
    countRetryableEntries,
    handleFileChange,
    handleImport,
    handleSync,
    toggleConfirmed,
    confirmAllPending,
    handleRetry,
    retryPreview,
    refreshLatestTask,
  } = useImportSyncOperations({
    addLog,
    resetLogScroll,
    startLogSession,
  });

  const operationRunning = importing || syncing;
  const liveProgress = importing && logMode === "import"
    ? [...logs].reverse().find((log) => typeof log.current === "number" && typeof log.total === "number" && log.total > 0)
    : undefined;

  // 使用当前交互阶段，不比较浏览器与服务器的时钟。
  const freshPreview =
    activeTab === "import" &&
    (previewLoading || !!previewError || (!!preview && !previewConsumed));
  const stage = operationRunning
    ? "running"
    : freshPreview
      ? "preview"
      : (activeTab === "import" ? lastTask : logs.length > 0 && logMode === "sync")
        ? "done"
        : "idle";


  return (
    <main className="import-main">
      <div className="import-workspace" data-stage={stage}>
        <section className="import-operation-panel" aria-label="导入与同步设置">
          <ImportPageTabs
            activeTab={activeTab}
            disabled={operationRunning}
            onChange={setActiveTab}
          />

          <div
            className="import-operation-content"
            role="tabpanel"
            id={`import-tabpanel-${activeTab}`}
            aria-labelledby={`import-tab-${activeTab}`}
            tabIndex={0}
          >
            {activeTab === "import" && (
              <ImportOpmlPanel
                file={file}
                disabled={operationRunning}
                importing={importing}
                preview={preview}
                previewLoading={previewLoading}
                previewError={previewError}
                canSubmitImport={canSubmitImport}
                confirmedUrls={confirmedUrls}
                lastTask={lastTask}
                taskEntries={taskEntries}
                latestTaskError={latestTaskError}
                hasNewPreview={Boolean(freshPreview)}
                liveProgress={liveProgress?.current !== undefined && liveProgress.total !== undefined ? { current: liveProgress.current, total: liveProgress.total } : undefined}
                onFileChange={handleFileChange}
                onImport={handleImport}
                onToggleConfirmed={toggleConfirmed}
                onConfirmAllPending={confirmAllPending}
                onRetry={handleRetry}
                onRetryPreview={retryPreview}
                onRetryLatestTask={() => {
                  void refreshLatestTask();
                }}
                countRetryableEntries={countRetryableEntries}
              />
            )}

            {activeTab === "sync" && (
              <SyncMetadataPanel
                disabled={operationRunning}
                syncing={syncing}
                onSync={handleSync}
              />
            )}
          </div>
        </section>

        <aside className="import-log-column" aria-label="操作日志">
          <details className="import-log-disclosure" open={syncing}>
            <summary>详细日志<span>{logs.length > 0 ? `${logs.length} 条记录` : "暂无本次日志"}</span></summary>
          <SyncLogPanel
            title={logPanelMode === "import" ? "导入日志" : "同步日志"}
            logs={logs}
            filteredLogs={filteredLogs}
            stats={stats}
            filter={filter}
            isRunning={operationRunning}
            autoScroll={autoScroll}
            onFilterChange={setFilter}
            onLogScroll={handleLogScroll}
            onResumeAutoScroll={resumeAutoScroll}
            onClearLogs={() => {
              clearLogSession(activeTab);
              resetLogScroll();
            }}
            logContainerRef={logContainerRef}
            logEndRef={logEndRef}
          />
          </details>
        </aside>
      </div>
    </main>
  );
}

// Wrapper component with PageLayout
export default function ImportPageClient({ initialTab = "import" }: { initialTab?: ImportTab }) {
  return (
    <PageLayout
      rootClassName="editorial-page-shell rhythm-page-shell"
      className="import-page"
      toolbar={{
        title: "导入/同步",
        className: "editorial-page-toolbar",
      }}
    >
      <ImportPageContent initialTab={initialTab} />
    </PageLayout>
  );
}
