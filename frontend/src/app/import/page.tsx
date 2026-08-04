"use client";

import { useState, useEffect } from "react";
import PageLayout from "@/components/layout/PageLayout";
import ImportOpmlPanel from "@/components/import/ImportOpmlPanel";
import ImportPageTabs, { type ImportTab } from "@/components/import/ImportPageTabs";
import SyncLogPanel from "@/components/import/SyncLogPanel";
import SyncMetadataPanel from "@/components/import/SyncMetadataPanel";
import { useImportSyncOperations } from "@/hooks/useImportSyncOperations";
import { useStableLogScroll } from "@/hooks/useStableLogScroll";
import { useSyncLogSession } from "@/hooks/useSyncLogSession";

function ImportPageContent() {
  const [activeTab, setActiveTab] = useState<ImportTab>("import");

  const {
    logs,
    logMode,
    filter,
    setFilter,
    restoredMode,
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
    handleFileChange,
    handleImport,
    handleSync,
  } = useImportSyncOperations({
    addLog,
    resetLogScroll,
    startLogSession,
  });

  const operationRunning = importing || syncing;

  useEffect(() => {
    if (restoredMode) {
      setActiveTab(restoredMode);
    }
  }, [restoredMode]);

  return (
    <main className="import-main">
      <div className="import-workspace">
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
                onFileChange={handleFileChange}
                onImport={handleImport}
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
        </aside>
      </div>
    </main>
  );
}

// Wrapper component with PageLayout
export default function ImportPage() {
  return (
    <PageLayout
      rootClassName="editorial-page-shell"
      className="import-page"
      toolbar={{
        title: "导入/同步",
        className: "editorial-page-toolbar",
      }}
    >
      <ImportPageContent />
    </PageLayout>
  );
}
