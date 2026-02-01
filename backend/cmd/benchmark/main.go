package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// BenchmarkResult 存储基准测试结果
type BenchmarkResult struct {
	Name       string
	Duration   time.Duration
	Requests   int
	Success    int
	Failed     int
	AvgTime    time.Duration
	MinTime    time.Duration
	MaxTime    time.Duration
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
	Throughput float64 // 请求/秒
}

// Config 基准测试配置
type Config struct {
	BaseURL  string        // API基础URL
	Workers  int           // 并发worker数
	Duration time.Duration // 测试持续时间
	Timeout  time.Duration // 请求超时时间
}

func main() {
	// 从环境变量读取配置，或使用默认值
	baseURL := getEnv("BENCHMARK_BASE_URL", "http://localhost:8080")
	workers := getEnvInt("BENCHMARK_WORKERS", 10)
	duration := getEnvDuration("BENCHMARK_DURATION", 30*time.Second)

	config := Config{
		BaseURL:  baseURL,
		Workers:  workers,
		Duration: duration,
		Timeout:  30 * time.Second,
	}

	log.Printf("开始性能基准测试...")
	log.Printf("配置: BaseURL=%s, Workers=%d, Duration=%v", config.BaseURL, config.Workers, config.Duration)

	// 检查服务是否可用
	if !checkHealth(config.BaseURL) {
		log.Fatalf("API服务不可用，请确保服务正在运行: %s", config.BaseURL)
	}

	// 运行所有基准测试
	results := []BenchmarkResult{}

	// 1. 健康检查端点
	log.Println("\n========== 测试: 健康检查 ==========")
	results = append(results, runBenchmark("健康检查", config, func() *http.Request {
		req, _ := http.NewRequest("GET", config.BaseURL+"/health", nil)
		return req
	}))

	// 2. 播客列表端点
	log.Println("\n========== 测试: 播客列表 ==========")
	results = append(results, runBenchmark("播客列表", config, func() *http.Request {
		req, _ := http.NewRequest("GET", config.BaseURL+"/api/v1/podcasts?page=1&page_size=20", nil)
		return req
	}))

	// 3. 搜索端点
	log.Println("\n========== 测试: 全文搜索 ==========")
	results = append(results, runBenchmark("全文搜索", config, func() *http.Request {
		req, _ := http.NewRequest("GET", config.BaseURL+"/api/v1/search?q=test&page=1&page_size=10", nil)
		return req
	}))

	// 4. 标签列表端点
	log.Println("\n========== 测试: 标签列表 ==========")
	results = append(results, runBenchmark("标签列表", config, func() *http.Request {
		req, _ := http.NewRequest("GET", config.BaseURL+"/api/v1/tags?page=1&page_size=20", nil)
		return req
	}))

	// 5. 工作流列表端点
	log.Println("\n========== 测试: 工作流列表 ==========")
	results = append(results, runBenchmark("工作流列表", config, func() *http.Request {
		req, _ := http.NewRequest("GET", config.BaseURL+"/api/v1/workflows?page=1&page_size=20", nil)
		return req
	}))

	// 打印汇总报告
	printSummary(results)

	// 生成Markdown格式报告
	generateMarkdownReport(results)
}

// runBenchmark 运行单个基准测试
func runBenchmark(name string, config Config, requestFunc func() *http.Request) BenchmarkResult {
	client := &http.Client{
		Timeout: config.Timeout,
	}

	// 用于统计的通道
	type timing struct {
		duration time.Duration
		success  bool
	}
	timings := make(chan timing, 10000)

	// 使用WaitGroup等待所有worker完成
	var wg sync.WaitGroup

	// 启动计时器
	startTime := time.Now()
	stopTime := startTime.Add(config.Duration)

	// 启动结果收集goroutine
	done := make(chan bool)
	var mu sync.Mutex
	requests := 0
	successCount := 0
	failCount := 0
	durations := []time.Duration{}

	go func() {
		for t := range timings {
			mu.Lock()
			if t.success {
				successCount++
				durations = append(durations, t.duration)
			} else {
				failCount++
			}
			requests++
			mu.Unlock()
		}
		done <- true
	}()

	// 启动worker池
	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for time.Now().Before(stopTime) {
				req := requestFunc()
				reqStart := time.Now()
				resp, err := client.Do(req)
				reqDuration := time.Since(reqStart)

				if err != nil || resp.StatusCode != http.StatusOK {
					timings <- timing{reqDuration, false}
				} else {
					timings <- timing{reqDuration, true}
					resp.Body.Close()
				}
			}
		}(i)
	}

	// 等待所有worker完成
	wg.Wait()
	close(timings)
	<-done

	// 统计结果
	totalDuration := time.Since(startTime)

	// 计算统计指标
	result := BenchmarkResult{
		Name:     name,
		Duration: totalDuration,
		Requests: requests,
		Success:  successCount,
		Failed:   failCount,
	}

	if len(durations) > 0 {
		result.AvgTime = avg(durations)
		result.MinTime = min(durations)
		result.MaxTime = max(durations)
		result.P50 = percentile(durations, 50)
		result.P95 = percentile(durations, 95)
		result.P99 = percentile(durations, 99)
		result.Throughput = float64(successCount) / totalDuration.Seconds()
	}

	// 打印结果
	fmt.Printf("\n结果:\n")
	fmt.Printf("  总请求数: %d\n", result.Requests)
	fmt.Printf("  成功: %d\n", result.Success)
	fmt.Printf("  失败: %d\n", result.Failed)
	fmt.Printf("  成功率: %.2f%%\n", float64(result.Success)/float64(result.Requests)*100)
	fmt.Printf("  吞吐量: %.2f 请求/秒\n", result.Throughput)
	fmt.Printf("  平均响应时间: %v\n", result.AvgTime)
	fmt.Printf("  最小响应时间: %v\n", result.MinTime)
	fmt.Printf("  最大响应时间: %v\n", result.MaxTime)
	fmt.Printf("  P50: %v\n", result.P50)
	fmt.Printf("  P95: %v\n", result.P95)
	fmt.Printf("  P99: %v\n", result.P99)

	return result
}

// printSummary 打印汇总报告
func printSummary(results []BenchmarkResult) {
	log.Println("\n" + strings.Repeat("=", 80))
	log.Println("性能基准测试汇总报告")
	log.Println(strings.Repeat("=", 80))

	fmt.Printf("\n%-20s %-10s %-10s %-12s %-10s %-10s %-10s\n",
		"测试名称", "请求总数", "成功率", "吞吐量(req/s)", "平均(ms)", "P95(ms)", "P99(ms)")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range results {
		fmt.Printf("%-20s %-10d %-10.1f %-12.2f %-10.1f %-10.1f %-10.1f\n",
			r.Name,
			r.Requests,
			float64(r.Success)/float64(r.Requests)*100,
			r.Throughput,
			float64(r.AvgTime.Milliseconds()),
			float64(r.P95.Milliseconds()),
			float64(r.P99.Milliseconds()))
	}
}

// generateMarkdownReport 生成Markdown格式报告
func generateMarkdownReport(results []BenchmarkResult) {
	report := fmt.Sprintf(`# 性能基准测试报告

__测试时间__: %s
__测试环境__: %s

## 测试结果摘要

| 测试名称 | 请求总数 | 成功率 | 吞吐量(req/s) | 平均(ms) | P50(ms) | P95(ms) | P99(ms) |
|---------|---------|--------|--------------|---------|---------|---------|---------|
`,
		time.Now().Format("2006-01-02 15:04:05"),
		getEnv("ENVIRONMENT", "development"))

	for _, r := range results {
		report += fmt.Sprintf("| %s | %d | %.1f%% | %.2f | %.1f | %.1f | %.1f | %.1f |\n",
			r.Name,
			r.Requests,
			float64(r.Success)/float64(r.Requests)*100,
			r.Throughput,
			float64(r.AvgTime.Milliseconds()),
			float64(r.P50.Milliseconds()),
			float64(r.P95.Milliseconds()),
			float64(r.P99.Milliseconds()))
	}

	report += fmt.Sprintf(`

## 详细分析

### 健康检查端点
- 吞吐量: %.2f req/s
- P95延迟: %v
- __目标__: P95 < 10ms

### 播客列表端点
- 吞吐量: %.2f req/s
- P95延迟: %v
- __目标__: P95 < 150ms

### 全文搜索端点
- 吞吐量: %.2f req/s
- P95延迟: %v
- __目标__: P95 < 200ms

### 标签列表端点
- 吞吐量: %.2f req/s
- P95延迟: %v
- __目标__: P95 < 100ms

### 工作流列表端点
- 吞吐量: %.2f req/s
- P95延迟: %v
- __目标__: P95 < 150ms

## 性能基线

本测试结果将作为重构前的性能基线。重构后的目标：

- API P95响应时间: __降低20%%__
- 吞吐量: __提升20%%__
- 成功率: __保持>99%%__

## 重构后对比

重构完成后，将使用相同的配置重新运行测试，并对比结果。

---

__生成时间__: %s
`,
		results[0].Throughput, results[0].P95,
		results[1].Throughput, results[1].P95,
		results[2].Throughput, results[2].P95,
		results[3].Throughput, results[3].P95,
		results[4].Throughput, results[4].P95,
		time.Now().Format("2006-01-02 15:04:05"))

	// 保存到文件
	filename := fmt.Sprintf("benchmark_results_%s.md", time.Now().Format("20060102_150405"))
	if err := os.WriteFile(filename, []byte(report), 0644); err != nil {
		log.Printf("警告: 无法保存报告文件: %v", err)
	} else {
		log.Printf("\n报告已保存到: %s", filename)
	}
}

// checkHealth 检查API服务健康状态
func checkHealth(baseURL string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// 辅助函数
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var i int
		if _, err := fmt.Sscanf(value, "%d", &i); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

func avg(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	return sum / time.Duration(len(durations))
}

func min(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	minimum := durations[0]
	for _, d := range durations {
		if d < minimum {
			minimum = d
		}
	}
	return minimum
}

func max(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	maximum := durations[0]
	for _, d := range durations {
		if d > maximum {
			maximum = d
		}
	}
	return maximum
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	// 复制并排序
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// 简单冒泡排序
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := int(float64(len(sorted)-1) * p / 100)
	return sorted[index]
}
