package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"magicpodcast/internal/logger"
	"magicpodcast/internal/personidentity"

	"github.com/gin-gonic/gin"
)

// Only this HTTP goroutine writes the response. Work and heartbeat events share
// one cancellable request; disconnecting cannot start a second preparation.
func streamPersonPreparation(c *gin.Context, ctx context.Context, id uint, prepare func(context.Context, uint) (personidentity.EpisodePeople, error)) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	requestID, err := newPersonRequestID()
	if err != nil {
		writePersonUnavailable(c)
		return
	}
	observation := personidentity.NewObservation(requestID, id)
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
	workCtx := personidentity.WithPreparationProgress(personidentity.WithObservation(ctx, observation), func(event personidentity.PreparationProgress) {
		select {
		case events <- event:
		case <-ctx.Done():
		}
	})
	// The worker owns observation until cancellation cleanup has finished.
	// Summarizing here avoids racing ctx.Done against its in-flight writes and
	// preserves a committed draft even when the client has already gone away.
	go func() {
		p, err := prepare(workCtx, id)
		observation.Finish(err)
		state := "completed"
		if err != nil {
			state = "failed"
			if errors.Is(err, context.Canceled) {
				state = "cancelled"
			}
		}
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			state = "client_disconnected"
		}
		var saved *bool
		if err == nil {
			value := p.Draft != nil
			saved = &value
		}
		logger.WithFields(observation.Summary(state, saved)).Info("person identity preparation summary")
		done <- outcome{p, err}
	}()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	emit := func(event personidentity.PreparationProgress) {
		if event.SourceVersion != "" {
			source = event.SourceVersion
		}
		if event.Runtime != nil {
			runtime := gin.H{"phase": event.Runtime.Phase}
			if event.Runtime.Classification != "" {
				runtime["classification"] = event.Runtime.Classification
			}
			if event.Runtime.LastActivityAgeMS != nil {
				runtime["last_activity_age_ms"] = *event.Runtime.LastActivityAgeMS
			}
			if event.Runtime.WillRetry != nil {
				runtime["will_retry"] = *event.Runtime.WillRetry
			}
			send("runtime", gin.H{"runtime": runtime})
			return
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
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					code, message := personPreparationError(ctx.Err())
					send("error", gin.H{"code": code, "message": message, "classification": personidentity.FailureDeadline, "retryable": true})
				}
				return
			}
			if result.err != nil {
				code, message := personPreparationError(result.err)
				class := personidentity.ClassifyFailure(result.err)
				send("error", gin.H{"code": code, "message": message, "classification": class.Code, "retryable": class.Retryable})
				return
			}
			source = result.people.SourceVersion
			send("complete", gin.H{"data": result.people})
			return
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				send("error", gin.H{
					"code":           "PERSON_PREPARATION_TIMEOUT",
					"message":        "识别超时，请核对已保存结果后重试。",
					"classification": personidentity.FailureDeadline,
					"retryable":      true,
				})
			}
			return
		}
	}
}

// newPersonRequestID returns a short opaque correlation token. It is long
// enough to stay unique across retries and short enough to read aloud or copy
// when reporting a problem.
func newPersonRequestID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// personPreparationError maps a preparation failure onto a stable SSE code and
// one safe user-facing message. Underlying messages are never forwarded: they
// may reference database or internals; the classification stays truthful.
func personPreparationError(err error) (string, string) {
	switch personidentity.ClassifyFailure(err).Code {
	case personidentity.FailureSourcesChanged:
		return "PERSON_SOURCE_CHANGED", "人物资料或逐字稿已更新，请重新读取后核对。"
	case personidentity.FailureDeadline:
		return "PERSON_PREPARATION_TIMEOUT", "识别超时，请核对已保存结果后重试。"
	case personidentity.FailureRuntimeUnavailable:
		return "PERSON_RUNTIME_UNAVAILABLE", "识别服务暂时无法连接，已有结果保留，可稍后重试。"
	case personidentity.FailureAuthentication:
		return "PERSON_AUTHENTICATION_FAILED", "识别服务认证失效，请检查账号授权；已有结果保留。"
	case personidentity.FailureQuota:
		return "PERSON_QUOTA_EXCEEDED", "识别服务额度或请求频率受限，请稍后再试；已有结果保留。"
	case personidentity.FailureConnection:
		return "PERSON_CONNECTION_FAILED", "识别服务上游连接失败，请核对已保存结果后重试。"
	case personidentity.FailureCancelled:
		return "PERSON_PREPARATION_CANCELLED", "已请求取消识别，请核对已保存结果。"
	case personidentity.FailureInvalidResult:
		return "PERSON_PREPARATION_FAILED", "识别结果未通过校验，请重试；已有结果保留。"
	case personidentity.FailureSaveFailed:
		return "PERSON_PREPARATION_FAILED", "草稿保存失败，请重试；已有结果保留。"
	case personidentity.FailureInvalidRequest:
		return "PERSON_PREPARATION_FAILED", "当前单集缺少可识别的逐字稿来源。"
	default:
		return "PERSON_PREPARATION_FAILED", "识别未完成，请核对已保存结果后重试。"
	}
}
