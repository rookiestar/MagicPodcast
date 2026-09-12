package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
)

// Only this HTTP goroutine writes the response. Work and heartbeat events share
// one cancellable request; disconnecting cannot start a second preparation.
func streamPersonPreparation(c *gin.Context, ctx context.Context, id uint, prepare func(context.Context, uint) (personidentity.EpisodePeople, error)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		writePersonUnavailable(c)
		return
	}
	requestID := hex.EncodeToString(token[:])
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	started := time.Now()
	source := ""
	send := func(kind string, fields gin.H) {
		fields["type"] = kind
		fields["episode_id"] = id
		fields["request_id"] = requestID
		fields["source_version"] = source
		fields["elapsed_ms"] = time.Since(started).Milliseconds()
		c.SSEvent("", fields)
		c.Writer.Flush()
	}
	events := make(chan personidentity.PreparationProgress, 8)
	type outcome struct {
		people personidentity.EpisodePeople
		err    error
	}
	done := make(chan outcome, 1)
	workCtx := personidentity.WithPreparationProgress(ctx, func(event personidentity.PreparationProgress) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	})
	go func() { p, err := prepare(workCtx, id); done <- outcome{p, err} }()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	emit := func(event personidentity.PreparationProgress) {
		if event.SourceVersion != "" {
			source = event.SourceVersion
		}
		send("stage", gin.H{"stage": event.Stage})
	}
	for {
		select {
		case event := <-events:
			emit(event)
		case <-ticker.C:
			send("heartbeat", gin.H{})
		case result := <-done:
			// Drain queued phase boundaries before the terminal event.
			for len(events) > 0 {
				emit(<-events)
			}
			if ctx.Err() != nil {
				return
			}
			if result.err != nil {
				code, message := "PERSON_PREPARATION_FAILED", "识别未完成，请核对已保存结果后重试。"
				if errors.Is(result.err, personidentity.ErrSourcesChanged) {
					code, message = "PERSON_SOURCE_CHANGED", "人物资料或逐字稿已更新，请重新读取后核对。"
				}
				send("error", gin.H{"code": code, "message": message})
			} else {
				source = result.people.SourceVersion
				send("complete", gin.H{"data": result.people})
			}
			return
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				send("error", gin.H{"code": "PERSON_PREPARATION_TIMEOUT", "message": "识别超时，请核对已保存结果后重试。"})
			}
			return
		}
	}
}
