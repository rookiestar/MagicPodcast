"use client";

import { useState, useEffect } from "react";
import PageLayout from "@/components/layout/PageLayout";
import SyncLogPanel from "@/components/import/SyncLogPanel";
import { useImportSyncOperations } from "@/hooks/useImportSyncOperations";
import { useStableLogScroll } from "@/hooks/useStableLogScroll";
import { useSyncLogSession } from "@/hooks/useSyncLogSession";

type TabType = "import" | "sync";

function ImportPageContent() {
  const [activeTab, setActiveTab] = useState<TabType>("import");

  // 共享的日志和UI状态
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

  useEffect(() => {
    if (restoredMode) {
      setActiveTab(restoredMode);
    }
  }, [restoredMode]);

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-6">
        {/* Main Card */}
        <div className="bg-white dark:bg-slate-800 rounded-lg shadow-sm">
          {/* Tabs */}
          <div className="border-b border-slate-200 dark:border-slate-700 px-6 py-5">
            <div className="inline-flex rounded-lg border border-slate-200 bg-slate-100 p-1 dark:border-slate-700 dark:bg-slate-900">
              <button
                type="button"
                onClick={() => setActiveTab("import")}
                disabled={importing || syncing}
                aria-pressed={activeTab === "import"}
                className={`min-h-[44px] rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
                  activeTab === "import"
                    ? "bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-blue-300"
                    : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                } ${importing || syncing ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
              >
                导入 OPML
              </button>
              <button
                type="button"
                onClick={() => setActiveTab("sync")}
                disabled={importing || syncing}
                aria-pressed={activeTab === "sync"}
                className={`min-h-[44px] rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
                  activeTab === "sync"
                    ? "bg-white text-blue-700 shadow-sm dark:bg-slate-800 dark:text-blue-300"
                    : "text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                } ${importing || syncing ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`}
              >
                同步元数据
              </button>
            </div>
          </div>

          {/* Content */}
          <div className="p-6">
            {/* Import Tab */}
            {activeTab === "import" && (
              <>
                {/* 说明部分 */}
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

                {/* 文件上传 */}
                <div className="mb-6">
                  <label className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-2">
                    选择OPML文件
                  </label>
                  <input
                    type="file"
                    accept=".opml,.xml"
                    onChange={handleFileChange}
                    disabled={importing || syncing}
                    className="block w-full text-sm text-slate-500 dark:text-slate-400
                      file:mr-4 file:py-2 file:px-4
                      file:rounded-lg file:border-0
                      file:text-sm file:font-semibold
                      file:bg-blue-50 file:text-blue-700
                      hover:file:bg-blue-100
                      disabled:file:bg-slate-100 disabled:file:text-slate-400
                      focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500
                    "
                  />
                  {file && (
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">
                      已选择: {file.name} ({(file.size / 1024).toFixed(2)} KB)
                    </p>
                  )}
                </div>

                {/* 导入按钮 */}
                <div className="mb-6">
                  <button
                    type="button"
                    onClick={handleImport}
                    disabled={!file || importing || syncing}
                    className={`min-h-[44px] rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
                      !file || importing || syncing
                        ? "cursor-not-allowed bg-slate-300 dark:bg-slate-700"
                        : "cursor-pointer bg-green-600 hover:bg-green-700 dark:hover:bg-green-700"
                    }`}
                  >
                    {importing ? "导入中..." : "开始导入"}
                  </button>
                </div>
              </>
            )}

            {/* Sync Tab */}
            {activeTab === "sync" && (
              <>
                {/* 说明部分 */}
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

                {/* 同步按钮 */}
                <div className="mb-6">
                  <button
                    type="button"
                    onClick={handleSync}
                    disabled={importing || syncing}
                    className={`min-h-[44px] rounded-lg px-6 py-2.5 text-sm font-medium text-white transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-500 ${
                      importing || syncing
                        ? "cursor-not-allowed bg-slate-300 dark:bg-slate-700"
                        : "cursor-pointer bg-blue-600 hover:bg-blue-700 dark:hover:bg-blue-700"
                    }`}
                  >
                    {syncing ? "同步中..." : "开始同步"}
                  </button>
                </div>
              </>
            )}

            <SyncLogPanel
              title={logPanelMode === "import" ? "导入日志" : "同步日志"}
              logs={logs}
              filteredLogs={filteredLogs}
              stats={stats}
              filter={filter}
              isRunning={importing || syncing}
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
