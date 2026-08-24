package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"magicpodcast/internal/codexruntime"
)

var smokeOutputSchema = json.RawMessage(`{
	"type":"object",
	"additionalProperties":false,
	"properties":{"message":{"type":"string","minLength":1}},
	"required":["message"]
}`)

var cancellationOutputSchema = json.RawMessage(`{
	"type":"object",
	"additionalProperties":false,
	"properties":{"message":{"type":"string","minLength":20000}},
	"required":["message"]
}`)

type smokeEvidence struct {
	SchemaVersion  int                             `json:"schema_version"`
	ObservedAt     time.Time                       `json:"observed_at"`
	Host           string                          `json:"host"`
	SDKVersion     string                          `json:"sdk_version"`
	RuntimeVersion string                          `json:"runtime_version"`
	Completed      executionEvidence               `json:"completed"`
	Cancelled      executionEvidence               `json:"cancelled"`
	IdentityUnique bool                            `json:"identity_unique"`
	Diagnostics    codexruntime.ProcessDiagnostics `json:"diagnostics"`
	OrphanFree     bool                            `json:"orphan_free"`
}

type executionEvidence struct {
	Status             codexruntime.ExecutionStatus    `json:"status"`
	CreateMilliseconds int64                           `json:"create_ms"`
	FirstDeltaMillis   *int64                          `json:"first_delta_ms,omitempty"`
	CompleteMillis     int64                           `json:"complete_ms"`
	EventCount         int                             `json:"event_count"`
	OutputDeltaCount   int                             `json:"output_delta_count"`
	SequenceValid      bool                            `json:"sequence_valid"`
	ResultShapeValid   bool                            `json:"result_shape_valid"`
	CancellationMethod codexruntime.CancellationMethod `json:"cancellation_method,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "codex runtime smoke failed:", err)
		os.Exit(1)
	}
}

func run() error {
	python := flag.String("python", "", "absolute path to the pinned venv Python")
	hostScript := flag.String(
		"host-script",
		"",
		"absolute path to runtime_host.py",
	)
	workRoot := flag.String(
		"work-root",
		"",
		"absolute managed working root",
	)
	evidencePath := flag.String(
		"evidence",
		"",
		"absolute path for sanitized JSON evidence",
	)
	timeout := flag.Duration("timeout", 3*time.Minute, "overall smoke timeout")
	flag.Parse()

	if !filepath.IsAbs(*python) ||
		!filepath.IsAbs(*hostScript) ||
		!filepath.IsAbs(*workRoot) ||
		!filepath.IsAbs(*evidencePath) {
		return errors.New("all path flags must be absolute")
	}
	if err := os.MkdirAll(*workRoot, 0o700); err != nil {
		return fmt.Errorf("create managed work root: %w", err)
	}
	completedDir, err := os.MkdirTemp(*workRoot, "completed-")
	if err != nil {
		return fmt.Errorf("create completed work directory: %w", err)
	}
	defer os.RemoveAll(completedDir)
	cancelledDir, err := os.MkdirTemp(*workRoot, "cancelled-")
	if err != nil {
		return fmt.Errorf("create cancelled work directory: %w", err)
	}
	defer os.RemoveAll(cancelledDir)

	host, err := codexruntime.NewProcessHost(codexruntime.ProcessHostConfig{
		Command:  []string{*python, *hostScript},
		WorkRoot: *workRoot,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cancelledSnapshot, cancelled, err := runCancelledSmoke(
		ctx,
		host,
		cancelledDir,
	)
	if err != nil {
		_ = host.Close(context.Background())
		return err
	}
	completedSnapshot, completed, err := runCompletedSmoke(
		ctx,
		host,
		completedDir,
	)
	if err != nil {
		_ = host.Close(context.Background())
		return err
	}
	closeCtx, closeCancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	err = host.Close(closeCtx)
	closeCancel()
	if err != nil {
		return fmt.Errorf("close runtime host: %w", err)
	}

	diagnostics := host.Diagnostics()
	hostname, _ := os.Hostname()
	evidence := smokeEvidence{
		SchemaVersion:  1,
		ObservedAt:     time.Now().UTC(),
		Host:           hostname,
		SDKVersion:     "0.147.0",
		RuntimeVersion: completedSnapshot.RuntimeVersion,
		Completed:      completed,
		Cancelled:      cancelled,
		IdentityUnique: completedSnapshot.ID != cancelledSnapshot.ID,
		Diagnostics:    diagnostics,
		OrphanFree: diagnostics.ActiveExecutions == 0 &&
			diagnostics.LiveProcessGroups == 0,
	}
	if !evidence.IdentityUnique || !evidence.OrphanFree {
		return errors.New("runtime isolation or orphan check failed")
	}
	if cancelledSnapshot.CancellationMethod !=
		codexruntime.CancellationNativeInterrupt {
		return fmt.Errorf(
			"native cancellation was not observed: %s",
			cancelledSnapshot.CancellationMethod,
		)
	}
	if err := writeEvidence(*evidencePath, evidence); err != nil {
		return err
	}
	fmt.Printf(
		"runtime smoke passed: completed=%s cancelled=%s orphan_free=%t\n",
		completed.Status,
		cancelled.Status,
		evidence.OrphanFree,
	)
	return nil
}

func runCompletedSmoke(
	ctx context.Context,
	host *codexruntime.ProcessHost,
	workDir string,
) (codexruntime.ExecutionSnapshot, executionEvidence, error) {
	started := time.Now()
	snapshot, err := host.CreateExecution(
		ctx,
		codexruntime.ExecutionRequest{
			Kind:             codexruntime.ExecutionKindSmoke,
			WorkingDirectory: workDir,
			Prompt: "Return the exact JSON message runtime-smoke-ok. " +
				"Do not call tools or inspect files.",
			OutputSchema: smokeOutputSchema,
		},
	)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	createdAt := time.Now()
	events, err := host.SubscribeExecution(ctx, snapshot.ID)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	evidence := collectEventEvidence(started, createdAt, events)
	final, err := host.GetExecution(ctx, snapshot.ID)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	evidence.Status = final.Status
	evidence.CompleteMillis = time.Since(started).Milliseconds()
	if final.Status != codexruntime.StatusCompleted {
		return final, evidence, fmt.Errorf(
			"completed smoke ended as %s: %s (%s)",
			final.Status,
			final.ErrorCode,
			final.SafeMessage,
		)
	}
	var result struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(final.Result, &result); err != nil ||
		strings.TrimSpace(result.Message) == "" {
		return final, evidence, errors.New(
			"completed smoke returned an invalid result shape",
		)
	}
	evidence.ResultShapeValid = true
	if evidence.OutputDeltaCount == 0 || !evidence.SequenceValid {
		return final, evidence, errors.New(
			"completed smoke did not produce a valid stream",
		)
	}
	return final, evidence, nil
}

func runCancelledSmoke(
	ctx context.Context,
	host *codexruntime.ProcessHost,
	workDir string,
) (codexruntime.ExecutionSnapshot, executionEvidence, error) {
	started := time.Now()
	snapshot, err := host.CreateExecution(
		ctx,
		codexruntime.ExecutionRequest{
			Kind:             codexruntime.ExecutionKindSmoke,
			WorkingDirectory: workDir,
			Prompt: "Prepare a JSON message with at least 20000 characters. " +
				"Do not call tools or inspect files.",
			OutputSchema: cancellationOutputSchema,
		},
	)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	createdAt := time.Now()
	cancelResult, err := host.CancelExecution(ctx, snapshot.ID)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	events, err := host.SubscribeExecution(ctx, snapshot.ID)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	evidence := collectEventEvidence(started, createdAt, events)
	final, err := host.GetExecution(ctx, snapshot.ID)
	if err != nil {
		return codexruntime.ExecutionSnapshot{}, executionEvidence{}, err
	}
	evidence.Status = final.Status
	evidence.CompleteMillis = time.Since(started).Milliseconds()
	evidence.CancellationMethod = cancelResult.Method
	if final.Status != codexruntime.StatusCancelled {
		return final, evidence, fmt.Errorf(
			"cancel smoke ended as %s: %s (%s)",
			final.Status,
			final.ErrorCode,
			final.SafeMessage,
		)
	}
	if !evidence.SequenceValid {
		return final, evidence, errors.New(
			"cancel smoke emitted an invalid event sequence",
		)
	}
	return final, evidence, nil
}

func collectEventEvidence(
	started time.Time,
	createdAt time.Time,
	events <-chan codexruntime.Event,
) executionEvidence {
	evidence := executionEvidence{
		CreateMilliseconds: createdAt.Sub(started).Milliseconds(),
		SequenceValid:      true,
	}
	var lastSequence uint64
	for event := range events {
		evidence.EventCount++
		if event.Sequence != lastSequence+1 {
			evidence.SequenceValid = false
		}
		lastSequence = event.Sequence
		if event.Type == codexruntime.EventOutputDelta {
			evidence.OutputDeltaCount++
			if evidence.FirstDeltaMillis == nil {
				value := time.Since(started).Milliseconds()
				evidence.FirstDeltaMillis = &value
			}
		}
	}
	return evidence
}

func writeEvidence(path string, evidence smokeEvidence) error {
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("encode smoke evidence: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create evidence directory: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write smoke evidence: %w", err)
	}
	return nil
}
