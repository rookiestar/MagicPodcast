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
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-6">
        <div className="rounded-lg bg-white shadow-sm dark:bg-slate-800">
          <ImportPageTabs
            activeTab={activeTab}
            disabled={operationRunning}
            onChange={setActiveTab}
          />

          <div className="p-6">
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
          </div>
        </div>
      </div>
    </main>
  );
}

// Wrapper component with PageLayout
export default function ImportPage() {
  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "导入/同步",
      }}
    >
      <ImportPageContent />
    </PageLayout>
  );
}
