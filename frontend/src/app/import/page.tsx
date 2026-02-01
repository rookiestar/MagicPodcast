"use client";

import { useState, useEffect, useRef, useMemo } from "react";
import Link from "next/link";
import { syncApi } from "@/lib/api";

// 日志类型定义
type LogType =
  | "info"
  | "success"
  | "error"
  | "progress"
  | "summary"
  | "skip_paid"
  | "skip_cert"
  | "skip_not_found"
  | "skip_no_update"
  | "skip_access_denied"
  | "skip_geo_blocked"
  | "skip_duplicate"
  | "skip_invalid"
  | "skip_other";

interface LogEntry {
  id: string;
  type: LogType;
  message: string;
  timestamp: string;
  current?: number;
  total?: number;
  reason?: string; // 跳过原因
  data?: any; // summary数据
}

type TabType = "import" | "sync";

// 统计数据接口
interface SyncStats {
  total: number;
  success: number;
  errors: number;
  skips: number;
  skipPaid: number;
  skipCert: number;
  skipNotFound: number;
  skipAccess: number;
  skipGeo: number;
  skipOther: number;
  skipNoUpdate: number;
  duration: string;
  fromSummary: boolean;
}

export default function ImportPage() {
  const [activeTab, setActiveTab] = useState<TabType>("import");

  // 导入OPML状态
  const [file, setFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);

  // 同步元数据状态
  const [syncing, setSyncing] = useState(false);

  // 共享的日志和UI状态
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [filter, setFilter] = useState<"all" | "errors" | "success" | "skips">(
    "all",
  );
  const [autoScroll, setAutoScroll] = useState(true);

  // 统计数据 - 独立的state
  const [stats, setStats] = useState<SyncStats>({
    total: 0,
    success: 0,
    errors: 0,
    skips: 0,
    skipPaid: 0,
    skipCert: 0,
    skipNotFound: 0,
    skipAccess: 0,
    skipGeo: 0,
    skipOther: 0,
    skipNoUpdate: 0,
    duration: "",
    fromSummary: false,
  });

  const logContainerRef = useRef<HTMLDivElement>(null);
  const logEndRef = useRef<HTMLDivElement>(null);

  // 自动滚动到底部
  useEffect(() => {
    if (!autoScroll) return;

    requestAnimationFrame(() => {
      if (logEndRef.current && autoScroll) {
        logEndRef.current.scrollIntoView({ behavior: "auto", block: "end" });
      }
    });
  }, [logs, autoScroll]);

  // 从localStorage恢复状态（仅执行一次）
  useEffect(() => {
    const savedLogs = localStorage.getItem("syncLogs");
    const savedSyncing = localStorage.getItem("syncing");
    const savedImporting = localStorage.getItem("importing");

    const restoredLogs: LogEntry[] = [];

    if (savedLogs) {
      try {
        const parsedLogs = JSON.parse(savedLogs);
        restoredLogs.push(...parsedLogs);
      } catch (e) {
        console.error("Failed to parse saved logs:", e);
      }
    }

    // 如果之前在同步中，添加提示
    if (savedSyncing === "true") {
      restoredLogs.push({
        id: Date.now().toString(),
        type: "info",
        message: "⚠️ 页面已刷新，上次同步状态已丢失",
        timestamp: new Date().toLocaleTimeString(),
      });
    }

    // 如果之前在导入中，添加提示
    if (savedImporting === "true") {
      restoredLogs.push({
        id: Date.now().toString(),
        type: "info",
        message: "⚠️ 页面已刷新，导入需要重新开始",
        timestamp: new Date().toLocaleTimeString(),
      });
    }

    if (restoredLogs.length > 0) {
      setLogs(restoredLogs);
    }
  }, []);

  // 保存状态到localStorage
  useEffect(() => {
    localStorage.setItem("syncLogs", JSON.stringify(logs));
  }, [logs]);

  useEffect(() => {
    localStorage.setItem("syncing", syncing.toString());
  }, [syncing]);

  useEffect(() => {
    localStorage.setItem("importing", importing.toString());
  }, [importing]);

  // 监听滚动事件
  useEffect(() => {
    const container = logContainerRef.current;
    if (!container) return;

    const handleScroll = () => {
      if (autoScroll) {
        setAutoScroll(false);
      }
    };

    container.addEventListener("scroll", handleScroll, { passive: true });
    return () => container.removeEventListener("scroll", handleScroll);
  }, [autoScroll]);

  // 恢复自动滚动
  const handleResumeAutoScroll = () => {
    setAutoScroll(true);
    requestAnimationFrame(() => {
      if (logEndRef.current) {
        logEndRef.current.scrollIntoView({ behavior: "smooth", block: "end" });
      }
    });
  };

  // 过滤后的日志
  const filteredLogs = logs.filter((log) => {
    if (filter === "all") return true;
    if (filter === "errors") return log.type === "error";
    if (filter === "success") {
      // success分类显示：success类型的消息，或者有单集更新的skip_no_update消息
      return (
        log.type === "success" ||
        (log.type === "skip_no_update" &&
          log.message &&
          log.message.includes("单集:"))
      );
    }
    if (filter === "skips") return log.type.startsWith("skip_");
    return true;
  });

  // 重置统计数据
  const resetStats = () => {
    setStats({
      total: 0,
      success: 0,
      errors: 0,
      skips: 0,
      skipPaid: 0,
      skipCert: 0,
      skipNotFound: 0,
      skipAccess: 0,
      skipGeo: 0,
      skipOther: 0,
      skipNoUpdate: 0,
      duration: "",
      fromSummary: false,
    });
  };

  const addLog = (
    type: LogType,
    message: string,
    current?: number,
    total?: number,
    data?: any,
  ) => {
    // 明确创建log对象，确保data字段被包含
    const newLog: LogEntry = {
      id: Date.now() + Math.random().toString(),
      type: type as LogEntry["type"],
      message: message,
      timestamp: new Date().toLocaleTimeString(),
    };

    // 显式添加可选字段，避免条件展开的问题
    if (current !== undefined) {
      newLog.current = current;
    }
    if (total !== undefined) {
      newLog.total = total;
    }
    if (data !== undefined) {
      newLog.data = data;
    }

    // 增量更新统计数据
    setStats((prev) => {
      const newStats = { ...prev };

      // 如果是summary消息，使用后端发送的准确统计
      if (type === "summary" && data) {
        console.log("[addLog] 收到summary，更新stats:", data);
        newStats.total = data.total_podcasts || 0;
        newStats.success = data.success_podcasts || 0;
        newStats.errors = data.failed_podcasts || 0;
        newStats.skips = data.skipped_podcasts || 0;
        newStats.skipNoUpdate = data.no_update_podcasts || 0;
        newStats.duration = data.duration || "";
        newStats.fromSummary = true;
        return newStats;
      }

      // 否则，根据消息类型增量更新
      if (type === "success") {
        // 任何success消息都计入成功数
        newStats.success++;
        newStats.total++;
        console.log("[addLog] success消息，stats+1:", message);
      } else if (type === "error") {
        newStats.errors++;
        newStats.total++;
      } else if (type.startsWith("skip_")) {
        newStats.skips++;
        newStats.total++;

        // 更新具体类型的跳过统计
        switch (type) {
          case "skip_paid":
            newStats.skipPaid++;
            break;
          case "skip_cert":
            newStats.skipCert++;
            break;
          case "skip_not_found":
            newStats.skipNotFound++;
            break;
          case "skip_access_denied":
            newStats.skipAccess++;
            break;
          case "skip_geo_blocked":
            newStats.skipGeo++;
            break;
          case "skip_no_update":
            newStats.skipNoUpdate++;
            break;
          default:
            newStats.skipOther++;
            break;
        }
      }

      return newStats;
    });

    // 如果是summary，打印保存的数据
    if (type === "summary") {
      console.log("[addLog] 保存summary log, data字段:", newLog.data);
      console.log("[addLog] 完整的log对象:", newLog);
    }

    setLogs((prev) => [...prev, newLog]);
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = e.target.files?.[0];
    if (selectedFile) {
      const validTypes = [
        "application/xml",
        "text/xml",
        "text/opml",
        "text/x-opml",
      ];
      const fileExt = selectedFile.name.split(".").pop()?.toLowerCase();

      if (
        !validTypes.includes(selectedFile.type) &&
        !["opml", "xml"].includes(fileExt || "")
      ) {
        alert("请选择OPML或XML文件");
        return;
      }

      setFile(selectedFile);
      setLogs([]);
    }
  };

  // 导入OPML（智能模式：本地匹配+在线同步）
  const handleImport = async () => {
    if (!file) {
      alert("请先选择OPML文件");
      return;
    }

    setImporting(true);
    setLogs([]);
    resetStats(); // 重置统计数据

    addLog("info", "开始导入OPML（智能模式：本地匹配+在线同步）...");

    let receivedSummary = false;

    try {
      await syncApi.importOPMLSSE(file, (type, message, current, total) => {
        addLog(type as LogType, message, current, total);
        // 标记是否收到了summary消息
        if (type === "summary" || type === "complete") {
          receivedSummary = true;
        }
      });

      // 如果导入成功完成但没有收到summary消息，添加一个默认的完成消息
      if (!receivedSummary) {
        console.log("[Import] 导入完成但未收到summary消息，添加默认完成消息");
        addLog("success", "✅ 导入完成！所有播客已自动同步");
      }
    } catch (error: any) {
      console.error("导入失败:", error);

      if (error.message?.includes("超时")) {
        addLog("error", "导入超时：可能是网络较慢或文件太大");
        addLog("info", "提示：您可以重新导入，系统会自动跳过已导入的播客");
      } else if (
        error.message?.includes("Network") ||
        error.message?.includes("fetch")
      ) {
        addLog("error", "网络连接错误：" + (error.message || "未知错误"));
        addLog("info", "提示：请检查网络连接后重试");
      } else if (
        error.message?.includes("abort") ||
        error.message?.includes("取消")
      ) {
        addLog("error", "导入被取消");
      } else {
        addLog("error", "导入失败：" + (error.message || "未知错误"));
        addLog("info", "提示：部分播客可能已成功导入，您可以查看播客列表");
      }
    } finally {
      setImporting(false);
    }
  };

  // 同步元数据
  const handleSync = async () => {
    setSyncing(true);
    setLogs([]); // 清空日志
    resetStats(); // 重置统计数据

    addLog("info", "开始同步所有播客的元数据...");

    let receivedSummary = false;

    try {
      await syncApi.syncPodcastsMetadataSSE(
        (type, message, current, total, data) => {
          console.log("[handleSync] 收到消息:", type, "data参数:", data);
          addLog(type as LogType, message, current, total, data);
          // 标记是否收到了summary消息
          if (type === "summary" || type === "complete") {
            receivedSummary = true;
          }
        },
      );

      // 如果同步成功完成但没有收到summary消息，添加一个默认的完成消息
      if (!receivedSummary) {
        console.log("[Sync] 同步完成但未收到summary消息，添加默认完成消息");
        addLog("success", "✅ 同步已完成！");
      }
    } catch (error: any) {
      console.error("同步失败:", error);
      addLog("error", "同步失败：" + (error.message || "未知错误"));
    } finally {
      setSyncing(false);
    }
  };

  const getLogIcon = (type: LogEntry["type"]) => {
    switch (type) {
      case "success":
        return "✅";
      case "error":
        return "❌";
      case "progress":
        return "⏳";
      case "summary":
        return "📊";
      case "skip_paid":
        return "💰";
      case "skip_cert":
        return "🔐";
      case "skip_not_found":
        return "🔍";
      case "skip_no_update":
        return "✓";
      case "skip_access_denied":
        return "🚫";
      case "skip_geo_blocked":
        return "🌍";
      case "skip_duplicate":
        return "🔄";
      case "skip_invalid":
        return "📄";
      case "skip_other":
        return "⏭️";
      default:
        return "ℹ️";
    }
  };

  const getLogColor = (type: LogEntry["type"]) => {
    switch (type) {
      case "success":
        return "text-green-700";
      case "error":
        return "text-red-700";
      case "progress":
        return "text-blue-700";
      case "summary":
        return "text-purple-700";
      case "skip_paid":
        return "text-yellow-700";
      case "skip_cert":
        return "text-orange-700";
      case "skip_not_found":
        return "text-gray-600";
      case "skip_no_update":
        return "text-gray-500";
      case "skip_access_denied":
        return "text-red-600";
      case "skip_geo_blocked":
        return "text-purple-700";
      case "skip_duplicate":
        return "text-cyan-700";
      case "skip_invalid":
        return "text-indigo-700";
      case "skip_other":
        return "text-gray-500";
      default:
        return "text-gray-700";
    }
  };

  const getLogTypeLabel = (type: LogEntry["type"]) => {
    switch (type) {
      case "skip_paid":
        return "付费播客";
      case "skip_cert":
        return "证书过期";
      case "skip_not_found":
        return "不存在";
      case "skip_no_update":
        return "无更新";
      case "skip_access_denied":
        return "访问拒绝";
      case "skip_geo_blocked":
        return "地区限制";
      case "skip_duplicate":
        return "重复";
      case "skip_invalid":
        return "格式无效";
      case "skip_other":
        return "其他";
      default:
        return "";
    }
  };

  return (
    <main className="min-h-screen bg-slate-50 dark:bg-slate-900">
      <div className="container mx-auto px-4 py-8">
        {/* Header */}
        <div className="mb-8">
          <div className="mb-4">
            <Link
              href="/"
              className="w-36 h-11 px-4 bg-white dark:bg-slate-700 text-slate-800 dark:text-slate-200 font-medium rounded-xl border border-slate-300 dark:border-slate-600 hover:bg-slate-50 dark:hover:bg-slate-600 hover:border-slate-400 dark:hover:border-slate-500 transition-colors flex items-center justify-center gap-2"
            >
              <span>←</span>
              <span>返回首页</span>
            </Link>
          </div>

          {/* 标题和描述 */}
          <div className="mb-4">
            <h1 className="text-4xl md:text-5xl font-semibold text-slate-800 dark:text-slate-50 mb-2">
              导入/同步
            </h1>
            <p className="text-base text-slate-600 dark:text-slate-400 max-w-2xl">
              从 OPML 文件导入播客或同步播客元数据
            </p>
          </div>
        </div>

        {/* Main Card */}
        <div className="bg-white dark:bg-slate-800 rounded-lg shadow-lg">
          {/* Tabs */}
          <div className="border-b border-slate-200 dark:border-slate-700 px-6 py-4">
            <div className="flex gap-6">
              <button
                onClick={() => setActiveTab("import")}
                disabled={importing || syncing}
                className={`pb-2 border-b-2 transition-colors text-base ${
                  activeTab === "import"
                    ? "border-blue-600 text-blue-600 dark:text-blue-400"
                    : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                } ${importing || syncing ? "opacity-50 cursor-not-allowed" : ""}`}
              >
                📁 导入OPML
              </button>
              <button
                onClick={() => setActiveTab("sync")}
                disabled={importing || syncing}
                className={`pb-2 border-b-2 transition-colors text-base ${
                  activeTab === "sync"
                    ? "border-blue-600 text-blue-600 dark:text-blue-400"
                    : "border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200"
                } ${importing || syncing ? "opacity-50 cursor-not-allowed" : ""}`}
              >
                🔄 同步元数据
              </button>
            </div>
          </div>

          {/* Content */}
          <div className="p-6">
            {/* Import Tab */}
            {activeTab === "import" && (
              <>
                {/* 说明部分 */}
                <div className="mb-6 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                  <h3 className="text-base font-medium text-blue-900 dark:text-blue-100 mb-2">
                    关于导入OPML
                  </h3>
                  <ul className="text-sm text-blue-800 dark:text-blue-200 space-y-1 list-disc list-inside">
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
                    disabled={importing}
                    className="block w-full text-sm text-slate-500 dark:text-slate-400
                      file:mr-4 file:py-2 file:px-4
                      file:rounded-lg file:border-0
                      file:text-sm file:font-semibold
                      file:bg-blue-50 file:text-blue-700
                      hover:file:bg-blue-100
                      disabled:file:bg-slate-100 disabled:file:text-slate-400
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
                    onClick={handleImport}
                    disabled={!file || importing}
                    className={`px-6 py-2.5 rounded-lg text-sm font-medium text-white transition-colors ${
                      !file || importing
                        ? "bg-slate-300 dark:bg-slate-700 cursor-not-allowed"
                        : "bg-green-600 hover:bg-green-700 dark:hover:bg-green-700"
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
                <div className="mb-6 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg">
                  <h3 className="text-base font-medium text-blue-900 dark:text-blue-100 mb-2">
                    关于同步元数据
                  </h3>
                  <ul className="text-sm text-blue-800 dark:text-blue-200 space-y-1 list-disc list-inside">
                    <li>从在线RSS feed更新所有播客的最新元数据</li>
                    <li>包括单集数量、最新发布时间、播客描述等信息</li>
                    <li>可能需要较长时间,取决于播客数量和网络状况</li>
                  </ul>
                </div>

                {/* 同步按钮 */}
                <div className="mb-6">
                  <button
                    onClick={handleSync}
                    disabled={syncing}
                    className={`px-6 py-2.5 rounded-lg text-sm font-medium text-white transition-colors ${
                      syncing
                        ? "bg-slate-300 dark:bg-slate-700 cursor-not-allowed"
                        : "bg-blue-600 hover:bg-blue-700 dark:hover:bg-blue-700"
                    }`}
                  >
                    {syncing ? "同步中..." : "开始同步"}
                  </button>
                </div>
              </>
            )}

            {/* 实时日志 */}
            {logs.length > 0 && (
              <div className="border border-slate-300 dark:border-slate-600 rounded-lg p-4 bg-slate-50 dark:bg-slate-900">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-3">
                    <h3 className="text-lg font-semibold text-slate-900 dark:text-slate-50">
                      {activeTab === "import" ? "导入日志" : "同步日志"}
                      {(importing || syncing) && (
                        <span className="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300">
                          进行中
                        </span>
                      )}
                    </h3>

                    {/* 过滤器按钮组 */}
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => setFilter("all")}
                        className={`text-sm px-4 py-1.5 rounded-lg border transition-colors min-w-[120px] ${
                          filter === "all"
                            ? "bg-blue-50 dark:bg-blue-900/30 border-blue-500 dark:border-blue-600 text-blue-700 dark:text-blue-300"
                            : "bg-white dark:bg-slate-800 border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
                        }`}
                      >
                        全部 ({stats.total})
                      </button>
                      <button
                        onClick={() => setFilter("success")}
                        className={`text-sm px-4 py-1.5 rounded-lg border transition-colors min-w-[120px] ${
                          filter === "success"
                            ? "bg-green-50 dark:bg-green-900/30 border-green-500 dark:border-green-600 text-green-700 dark:text-green-300"
                            : "bg-white dark:bg-slate-800 border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
                        }`}
                      >
                        成功 ({stats.success})
                      </button>
                      <button
                        onClick={() => setFilter("errors")}
                        className={`text-sm px-4 py-1.5 rounded-lg border transition-colors min-w-[120px] ${
                          filter === "errors"
                            ? "bg-red-50 dark:bg-red-900/30 border-red-500 dark:border-red-600 text-red-700 dark:text-red-300"
                            : "bg-white dark:bg-slate-800 border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
                        }`}
                      >
                        失败 ({stats.errors})
                      </button>
                      <button
                        onClick={() => setFilter("skips")}
                        className={`text-sm px-4 py-1.5 rounded-lg border transition-colors min-w-[120px] ${
                          filter === "skips"
                            ? "bg-slate-100 dark:bg-slate-800 border-slate-500 dark:border-slate-600 text-slate-700 dark:text-slate-300"
                            : "bg-white dark:bg-slate-800 border-slate-300 dark:border-slate-600 text-slate-700 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-700"
                        }`}
                      >
                        跳过 ({stats.skips})
                      </button>
                    </div>

                    {/* 自动滚动指示器和清空按钮 */}
                    <div className="flex items-center gap-3">
                      {!autoScroll && (
                        <button
                          onClick={handleResumeAutoScroll}
                          className="text-sm text-blue-600 dark:text-blue-400 hover:text-blue-800 dark:hover:text-blue-300"
                          title="恢复自动滚动"
                        >
                          ↺ 恢复自动滚动
                        </button>
                      )}
                      {!importing && !syncing && (
                        <button
                          onClick={() => {
                            setLogs([]);
                            setFilter("all");
                            setAutoScroll(true);
                            localStorage.removeItem("syncLogs");
                          }}
                          className="text-sm text-slate-500 dark:text-slate-400 hover:text-slate-700 dark:hover:text-slate-300"
                        >
                          清空日志
                        </button>
                      )}
                    </div>
                  </div>
                </div>

                {/* 统计信息 */}
                {(stats.total > 0 ||
                  stats.errors > 0 ||
                  stats.success > 0 ||
                  stats.skips > 0) && (
                  <div className="mb-4">
                    {/* 时间统计 */}
                    {stats.fromSummary && stats.duration && (
                      <div className="mb-3 text-xs text-blue-600 dark:text-blue-400 font-medium">
                        ⏱️ 总耗时: {stats.duration}
                      </div>
                    )}

                    {/* 统计卡片 */}
                    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                      {/* 总计 */}
                      <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                        <p className="text-2xl font-bold text-slate-900 dark:text-slate-50">
                          {stats.total}
                        </p>
                        <p className="text-sm text-slate-600 dark:text-slate-400">
                          总计
                        </p>
                      </div>

                      {/* 成功 */}
                      <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                        <p className="text-2xl font-bold text-green-600 dark:text-green-400">
                          {stats.success}
                        </p>
                        <p className="text-sm text-slate-600 dark:text-slate-400">
                          成功
                        </p>
                      </div>

                      {/* 失败 */}
                      <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                        <p className="text-2xl font-bold text-red-600 dark:text-red-400">
                          {stats.errors}
                        </p>
                        <p className="text-sm text-slate-600 dark:text-slate-400">
                          失败
                        </p>
                      </div>

                      {/* 跳过 */}
                      <div className="bg-slate-50 dark:bg-slate-900 rounded-lg p-4">
                        <p className="text-2xl font-bold text-slate-600 dark:text-slate-400">
                          {stats.skips}
                        </p>
                        <p className="text-sm text-slate-600 dark:text-slate-400">
                          跳过
                        </p>
                      </div>
                    </div>

                    {/* 详细跳过统计 */}
                    {stats.skips > 0 && (
                      <div className="mt-3 text-xs text-slate-600 dark:text-slate-400 flex flex-wrap gap-x-3">
                        <span>💰 付费: {stats.skipPaid}</span>
                        <span>🔐 证书: {stats.skipCert}</span>
                        <span>🔍 不存在: {stats.skipNotFound}</span>
                        <span>🚫 访问拒绝: {stats.skipAccess}</span>
                        <span>🌍 地区限制: {stats.skipGeo}</span>
                        <span>⏭️ 其他: {stats.skipOther}</span>
                      </div>
                    )}

                    {/* 无更新统计 */}
                    {stats.skipNoUpdate > 0 && (
                      <div className="mt-2 text-xs text-slate-500 dark:text-slate-400">
                        ✓ 无更新: {stats.skipNoUpdate} 个
                      </div>
                    )}
                  </div>
                )}

                <div
                  ref={logContainerRef}
                  className="space-y-1 max-h-96 overflow-y-auto"
                >
                  {filteredLogs.map((log) => (
                    <div
                      key={log.id}
                      className={`text-xs ${getLogColor(log.type)} font-mono`}
                    >
                      <span className="text-gray-400">[{log.timestamp}]</span>{" "}
                      <span>{getLogIcon(log.type)}</span>{" "}
                      <span>
                        {log.type === "progress" &&
                        log.current !== undefined &&
                        log.total !== undefined
                          ? `[${log.current}/${log.total}] `
                          : ""}
                        {log.type.startsWith("skip_") && (
                          <span className="font-semibold">
                            [{getLogTypeLabel(log.type)}]{" "}
                          </span>
                        )}
                        {log.type === "summary" && log.data ? (
                          // summary类型：显示详细的统计信息
                          <span className="font-semibold">
                            📊 同步完成！
                            <br />
                            播客统计: 总计 {log.data.total_podcasts} | 成功{" "}
                            {log.data.success_podcasts} | 失败{" "}
                            {log.data.failed_podcasts} | 跳过{" "}
                            {log.data.skipped_podcasts}
                            {log.data.no_update_podcasts > 0 && (
                              <span>
                                {" "}
                                (无更新: {log.data.no_update_podcasts})
                              </span>
                            )}
                            <br />
                            单集统计: 总处理 {log.data.total_episodes || 0} |
                            新增 {log.data.new_episodes || 0} | 更新{" "}
                            {log.data.updated_episodes || 0}
                            {log.data.duration && (
                              <span> | 耗时: {log.data.duration}</span>
                            )}
                          </span>
                        ) : (
                          // 其他类型：显示原始消息
                          log.message
                        )}
                      </span>
                    </div>
                  ))}
                  <div ref={logEndRef} />
                </div>

                {/* 自动滚动提示 */}
                {(importing || syncing) && autoScroll && (
                  <div className="mt-2 text-xs text-gray-500 text-center flex items-center justify-center">
                    <span className="inline-block animate-pulse mr-1">⬇</span>
                    正在处理中，自动滚动...
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </main>
  );
}
