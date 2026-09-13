package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"magicpodcast/internal/models"
	syncpkg "magicpodcast/internal/sync"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// failingResponseWriter 模拟客户端断开后的写出失败。
type failingResponseWriter struct{}

func (failingResponseWriter) Header() http.Header { return http.Header{} }
func (failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("client gone")
}
func (failingResponseWriter) WriteHeader(int) {}
func (failingResponseWriter) Flush()          {}

// TestSSEProgressReporterCloseReleasesResourcesAfterWriteFailure 验证写出
// 失败只停业务写路径；Close 仍必须释放 keepalive 资源且可重复调用（R13）。
func TestSSEProgressReporterCloseReleasesResourcesAfterWriteFailure(t *testing.T) {
	reporter := &SSEProgressReporter{
		flusher:       failingResponseWriter{},
		writer:        failingResponseWriter{},
		stopKeepalive: make(chan struct{}),
	}

	reporter.Report("这条消息会写出失败")
	assert.True(t, reporter.closed, "写出失败后停止业务写路径")

	select {
	case <-reporter.stopKeepalive:
		t.Fatal("写出失败本身不应释放资源，应留给 Close")
	default:
	}

	reporter.Close()
	select {
	case <-reporter.stopKeepalive:
	default:
		t.Fatal("Close 必须关闭 stopKeepalive 以退出保活 goroutine")
	}

	assert.NotPanics(t, func() { reporter.Close() }, "重复 Close 不得 panic 或重复关闭 channel")
}

// newImportTaskRouter 构建带隔离库与任务表的导入路由。
func newImportTaskRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Chdir(t.TempDir())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&models.Podcast{}, &models.ImportTask{}))
	service, err := syncpkg.NewService(db, "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, service.Close()) })
	handler := &SyncHandler{syncService: service, db: db}
	router := gin.New()
	router.POST("/import", handler.ImportOPMLSSE)
	router.POST("/tasks/:id/retry", handler.RetryImportTask)
	router.GET("/tasks/:id", handler.GetImportTaskStatus)
	router.GET("/tasks/latest", handler.GetLatestImportTask)
	return router, db
}

func postOPMLFile(t *testing.T, router *gin.Engine, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("opml_file", "task.opml")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-MagicPodcast-Confirmation", "IMPORT OPML")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

// TestImportTaskPersistsAcrossDisconnects 验证任务先保存终态与逐条结果：
// SSE 写出失败（客户端断开）不影响任务记录，状态查询与最终汇总一致（R4）。
func TestImportTaskPersistsAcrossDisconnects(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	router, db := newImportTaskRouter(t)

	opmlContent := `<opml version="2.0"><body><outline title="Task Show" text="d" type="rss" xmlUrl="` + upstream.URL + `/f"/></body></opml>`
	response := postOPMLFile(t, router, []byte(opmlContent))
	t.Logf("STATUS=%d BODY=%s", response.Code, response.Body.String())
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var taskCount int64
	require.NoError(t, db.Model(&models.ImportTask{}).Count(&taskCount).Error)
	require.Equal(t, int64(1), taskCount)

	var task models.ImportTask
	require.NoError(t, db.First(&task).Error)
	require.Equal(t, models.ImportTaskStatusCompleted, task.Status)
	require.Equal(t, 1, task.Total)
	require.Equal(t, 1, task.PendingCount, "不可达订阅应记录为待同步")
	require.NotEmpty(t, task.ResultJSON, "逐条结果必须持久化，不受日志截断影响")

	// 状态查询与最终汇总一致。
	statusResponse := httptest.NewRecorder()
	router.ServeHTTP(statusResponse, httptest.NewRequest(http.MethodGet, "/tasks/1", nil))
	require.Equal(t, http.StatusOK, statusResponse.Code)
	var payload struct {
		Task    models.ImportTask           `json:"task"`
		Entries []syncpkg.ImportEntryResult `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(statusResponse.Body.Bytes(), &payload))
	require.Equal(t, models.ImportTaskStatusCompleted, payload.Task.Status)
	require.Len(t, payload.Entries, 1)
	assert.Equal(t, syncpkg.ImportOutcomePending, payload.Entries[0].Outcome)

	// latest 端点返回同一任务。
	latestResponse := httptest.NewRecorder()
	router.ServeHTTP(latestResponse, httptest.NewRequest(http.MethodGet, "/tasks/latest", nil))
	require.Equal(t, http.StatusOK, latestResponse.Code)
	require.Contains(t, latestResponse.Body.String(), models.ImportTaskStatusCompleted)
}

// TestInterruptedImportTaskIsExplicitlyReported 验证进程重启后 running 的
// 任务被明确报告为中断/待继续，且不会自动重跑任何写入。
func TestInterruptedImportTaskIsExplicitlyReported(t *testing.T) {
	router, db := newImportTaskRouter(t)

	task, err := syncpkg.CreateImportTask(db, "crash.opml", 3)
	require.NoError(t, err)
	// 模拟进程重启：注册表清空后，running 记录不再被视为活动任务。
	syncpkg.ActiveImportTasks.Delete(task.ID)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/latest", nil))
	require.Equal(t, http.StatusOK, response.Code)

	var payload struct {
		Task models.ImportTask `json:"task"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, models.ImportTaskStatusInterrupted, payload.Task.Status)
}

// TestRetryImportTaskOnlyTouchesFailedAndPendingEntries 验证仅重试失败/
// 待同步条目：成功与冲突条目不重复执行，重试结果生成新任务（R12/#404）。
func TestRetryImportTaskOnlyTouchesFailedAndPendingEntries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	router, db := newImportTaskRouter(t)

	task, err := syncpkg.CreateImportTask(db, "retry.opml", 3)
	require.NoError(t, err)
	require.NoError(t, syncpkg.FinalizeImportTask(db, task, &syncpkg.SyncResult{
		TotalPodcasts: 3,
		Entries: []syncpkg.ImportEntryResult{
			{Title: "Pending Show", FeedURL: upstream.URL + "/pending.xml", Outcome: syncpkg.ImportOutcomePending},
			{Title: "Done Show", FeedURL: upstream.URL + "/done.xml", Outcome: syncpkg.ImportOutcomeUnchanged},
			{Title: "Conflict Show", FeedURL: upstream.URL + "/conflict.xml", Outcome: syncpkg.ImportOutcomeConflict},
		},
	}, nil))

	body := "confirmation_text=RETRY+IMPORT"
	req := httptest.NewRequest(http.MethodPost, "/tasks/"+strconv.FormatUint(uint64(task.ID), 10)+"/retry", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"total_podcasts":1`, "只有 1 条待同步条目参与重试")
	require.Contains(t, response.Body.String(), `"stub_podcasts":1`)

	var retryCount int64
	require.NoError(t, db.Model(&models.ImportTask{}).Count(&retryCount).Error)
	require.Equal(t, int64(2), retryCount, "重试生成新任务记录")
}
