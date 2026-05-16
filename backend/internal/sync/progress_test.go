package sync

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEProgressReporterConcurrentWrites(t *testing.T) {
	var buffer bytes.Buffer
	reporter := NewSSEProgressReporter(&buffer)

	var wg sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				reporter.ReportProgress(i+1, 10, "同步进度")
				reporter.ReportSuccess("同步成功")
				reporter.ReportError("同步失败")
			}
		}(worker)
	}

	wg.Wait()
	reporter.Close()
	reporter.Close()

	lines := strings.Split(buffer.String(), "\n")
	doneCount := 0
	dataCount := 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		require.True(t, strings.HasPrefix(line, "data: "), "invalid SSE line: %s", line)

		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			doneCount++
			continue
		}

		var message SSEMessage
		require.NoError(t, json.Unmarshal([]byte(payload), &message))
		dataCount++
	}

	assert.Equal(t, 1, doneCount)
	assert.Greater(t, dataCount, 0)
}

func TestSSEProgressReporterIgnoresWritesAfterClose(t *testing.T) {
	var buffer bytes.Buffer
	reporter := NewSSEProgressReporter(&buffer)

	reporter.Report("开始")
	reporter.Close()
	before := buffer.String()

	reporter.Report("不应该出现")
	reporter.ReportSummary(&SyncSummary{TotalPodcasts: 1})
	reporter.Close()

	assert.Equal(t, before, buffer.String())
	assert.Contains(t, before, "data: [DONE]")
}

func TestSSEProgressReporterUsesStableNoUpdateType(t *testing.T) {
	var buffer bytes.Buffer
	reporter := NewSSEProgressReporter(&buffer)

	reporter.ReportSkip(SkipReasonNoUpdate, "没有更新")
	reporter.ReportSkip(SkipReasonNoUpdate, "没有更新")
	reporter.ReportSkip(SkipReasonNoUpdate, "没有更新")
	reporter.Close()

	assert.Contains(t, buffer.String(), `"type":"skip_no_update"`)
	assert.NotContains(t, buffer.String(), "skip_noupd")
}

func TestTruncateMessagePreservesMultibyteCharacters(t *testing.T) {
	message := strings.Repeat("同步🚀", 200)

	truncated := truncateMessage(message, 20)

	assert.NotContains(t, truncated, "\uFFFD")
	assert.LessOrEqual(t, len([]rune(truncated)), 20)
}
