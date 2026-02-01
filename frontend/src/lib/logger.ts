/**
 * 统一的日志工具
 * 在生产环境禁用详细日志
 */

const isDevelopment = process.env.NODE_ENV === "development";

type LogLevel = "log" | "info" | "warn" | "error" | "debug";

class Logger {
  private prefix: string;

  constructor(prefix: string = "") {
    this.prefix = prefix;
  }

  private logMessage(level: LogLevel, ...args: any[]) {
    if (isDevelopment) {
      const message = this.prefix ? `[${this.prefix}]` : "";
      console[level](message, ...args);
    }
  }

  log(...args: any[]) {
    this.logMessage("log", ...args);
  }

  info(...args: any[]) {
    this.logMessage("info", ...args);
  }

  warn(...args: any[]) {
    this.logMessage("warn", ...args);
  }

  error(...args: any[]) {
    // 错误日志始终输出
    const message = this.prefix ? `[${this.prefix}]` : "";
    console.error(message, ...args);
  }

  debug(...args: any[]) {
    if (isDevelopment) {
      const message = this.prefix ? `[${this.prefix} Debug]` : "";
      console.log(message, ...args);
    }
  }
}

// 创建预定义的logger实例
export const apiLogger = new Logger("API");
export const syncLogger = new Logger("Sync");
export const uiLogger = new Logger("UI");
export const workflowLogger = new Logger("Workflow");

// 默认导出
export default Logger;
