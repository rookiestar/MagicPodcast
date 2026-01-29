package logger

import (
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log        *logrus.Logger
	customHook *logrusHook
	initOnce   sync.Once
)

// logrusHook 自定义hook，用于添加文件和行号信息
type logrusHook struct {
	io.Writer
}

// Fire 实现Hook接口
func (hook *logrusHook) Fire(entry *logrus.Entry) error {
	line, _ := entry.String()
	_, err := hook.Writer.Write([]byte(line))
	return err
}

// Levels 返回需要处理的日志级别
func (hook *logrusHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Init 初始化日志系统
func Init(logLevel string, logFile string, environment string) {
	log = logrus.New()

	// 设置日志级别
	switch logLevel {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	// 设置格式化器
	if environment == "release" || environment == "production" {
		// 生产环境使用JSON格式
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		// 开发环境使用文本格式，带颜色和文件名
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
			ForceColors:     true,
		})
	}

	// 设置输出
	var writers []io.Writer

	// 控制台输出
	writers = append(writers, os.Stdout)

	// 文件输出（如果指定了日志文件）
	if logFile != "" {
		// 确保日志目录存在
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Warnf("无法创建日志目录 %s: %v", logDir, err)
		} else {
			// 使用lumberjack进行日志轮转
			fileWriter := &lumberjack.Logger{
				Filename:   logFile,
				MaxSize:    100,  // MB
				MaxBackups: 3,    // 保留3个备份
				MaxAge:     28,   // 保留28天
				Compress:   true, // 压缩旧日志
			}
			writers = append(writers, fileWriter)
		}
	}

	// 多输出
	multiWriter := io.MultiWriter(writers...)
	log.SetOutput(multiWriter)

	// 添加自定义hook
	if environment != "release" && environment != "production" {
		customHook = &logrusHook{Writer: multiWriter}
		log.AddHook(customHook)
	}

	// 设置报告调用者（获取文件名和行号）
	log.SetReportCaller(environment == "debug")

	log.Info("日志系统初始化完成")
}

func ensureInitialized() {
	if log != nil {
		return
	}
	initOnce.Do(func() {
		l := logrus.New()
		l.SetLevel(logrus.InfoLevel)
		l.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
		l.SetOutput(os.Stdout)
		log = l
	})
}

// WithFields 返回带有字段的logger
func WithFields(fields map[string]interface{}) *logrus.Entry {
	ensureInitialized()
	return log.WithFields(fields)
}

// Debug 记录DEBUG级别日志
func Debug(args ...interface{}) {
	ensureInitialized()
	log.Debug(args...)
}

// Debugf 记录DEBUG级别格式化日志
func Debugf(format string, args ...interface{}) {
	ensureInitialized()
	log.Debugf(format, args...)
}

// Info 记录INFO级别日志
func Info(args ...interface{}) {
	ensureInitialized()
	log.Info(args...)
}

// Infof 记录INFO级别格式化日志
func Infof(format string, args ...interface{}) {
	ensureInitialized()
	log.Infof(format, args...)
}

// Warn 记录WARN级别日志
func Warn(args ...interface{}) {
	ensureInitialized()
	log.Warn(args...)
}

// Warnf 记录WARN级别格式化日志
func Warnf(format string, args ...interface{}) {
	ensureInitialized()
	log.Warnf(format, args...)
}

// Error 记录ERROR级别日志
func Error(args ...interface{}) {
	ensureInitialized()
	log.Error(args...)
}

// Errorf 记录ERROR级别格式化日志
func Errorf(format string, args ...interface{}) {
	ensureInitialized()
	log.Errorf(format, args...)
}

// Fatal 记录FATAL级别日志并退出程序
func Fatal(args ...interface{}) {
	ensureInitialized()
	log.Fatal(args...)
}

// Fatalf 记录FATAL级别格式化日志并退出程序
func Fatalf(format string, args ...interface{}) {
	ensureInitialized()
	log.Fatalf(format, args...)
}

// GetLogger 获取原始logger实例（用于需要直接使用的场景）
func GetLogger() *logrus.Logger {
	ensureInitialized()
	return log
}
