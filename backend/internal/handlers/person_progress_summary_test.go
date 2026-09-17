package handlers

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"magicpodcast/internal/logger"
	"magicpodcast/internal/personidentity"
	"net/http/httptest"
	"testing"
	"time"
)

type summaryHook struct{ entries chan logrus.Fields }

func (h *summaryHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *summaryHook) Fire(e *logrus.Entry) error {
	if e.Data["event"] == "person_identity_summary" {
		h.entries <- e.Data
	}
	return nil
}

// A disconnect cannot summarize an execution before its worker has settled.
// In particular it must not claim no draft while a committed draft is returned.
func TestPersonSummaryWaitsForCancelledWorkerAndPreservesSavedDraft(t *testing.T) {
	log := logger.GetLogger()
	old := log.ReplaceHooks(make(logrus.LevelHooks))
	defer log.ReplaceHooks(old)
	hook := &summaryHook{entries: make(chan logrus.Fields, 2)}
	log.AddHook(hook)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	go func() {
		defer close(returned)
		streamPersonPreparation(c, ctx, 7, func(work context.Context, _ uint) (personidentity.EpisodePeople, error) {
			close(started)
			<-work.Done()
			<-release
			personidentity.ObservationFrom(work).SourceVersion = "v-review"
			return personidentity.EpisodePeople{Draft: &personidentity.ReviewDraft{}}, nil
		})
	}()
	<-started
	cancel()
	<-returned
	select {
	case <-hook.entries:
		close(release)
		t.Fatal("summary emitted before cancellation cleanup settled")
	default:
	}
	close(release)
	select {
	case fields := <-hook.entries:
		require.Equal(t, "client_disconnected", fields["outcome"])
		require.Equal(t, true, fields["draft_saved"])
		require.Equal(t, "v-review", fields["source_version"])
		require.Contains(t, fields, "total_ms")
	case <-time.After(3 * time.Second):
		t.Fatal("worker summary missing")
	}
}
