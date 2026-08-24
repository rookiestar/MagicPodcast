package codexruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultStartupTimeout   = 20 * time.Second
	defaultNativeCancelWait = 3 * time.Second
	defaultTerminateWait    = 2 * time.Second
	defaultKillWait         = 2 * time.Second
	defaultPostExitGrace    = 250 * time.Millisecond
	defaultMaxFrameBytes    = 8 << 20
	defaultMaxEventBytes    = 8 << 20
	defaultMaxEvents        = 4096
	defaultMaxPromptBytes   = 4 << 20
	defaultMaxSchemaBytes   = 256 << 10
	defaultMaxResultBytes   = 2 << 20
	defaultSubscriberBuffer = 16
	defaultMaxRetained      = 256
	defaultResultReadGrace  = 30 * time.Second
)

type ProcessHostConfig struct {
	Command               []string
	WorkRoot              string
	Profiles              map[ExecutionKind]Profile
	Capabilities          map[string]string
	Environment           map[string]string
	StartupTimeout        time.Duration
	NativeCancelTimeout   time.Duration
	TerminateTimeout      time.Duration
	KillTimeout           time.Duration
	PostExitGrace         time.Duration
	MaxFrameBytes         int
	MaxEventBytes         int
	MaxEvents             int
	MaxPromptBytes        int
	MaxSchemaBytes        int
	MaxResultBytes        int
	SubscriberBufferSize  int
	MaxRetainedExecutions int
	ResultReadGrace       time.Duration
}

func DefaultProfiles() map[ExecutionKind]Profile {
	return map[ExecutionKind]Profile{
		ExecutionKindEpisodeNotes: {
			Sandbox: SandboxReadOnly,
		},
		ExecutionKindAssistant: {
			Sandbox:      SandboxReadOnly,
			AllowedTools: []ToolCapability{ToolWebSearch},
		},
		ExecutionKindSmoke: {
			Sandbox: SandboxReadOnly,
		},
	}
}

func NewProcessHost(config ProcessHostConfig) (*ProcessHost, error) {
	normalized, err := normalizeProcessHostConfig(config)
	if err != nil {
		return nil, err
	}
	return &ProcessHost{
		config:     normalized,
		executions: make(map[ExecutionID]*managedExecution),
	}, nil
}

type ProcessHost struct {
	config ProcessHostConfig

	mu         sync.RWMutex
	executions map[ExecutionID]*managedExecution
	closed     bool
}

type ProcessDiagnostics struct {
	TrackedExecutions int `json:"tracked_executions"`
	ActiveExecutions  int `json:"active_executions"`
	LiveProcessGroups int `json:"live_process_groups"`
}

type managedExecution struct {
	mu       sync.Mutex
	snapshot ExecutionSnapshot
	events   []Event
	bytes    int
	notify   chan struct{}

	command *exec.Cmd
	stdin   *os.File
	pid     int

	writeMu sync.Mutex

	lastSequence    uint64
	startedClosed   bool
	terminalClosed  bool
	nativeAckClosed bool
	processClosed   bool
	terminalFrame   bool
	cancelRequested bool
	subscribers     int
	pendingReads    int
	retainUntil     time.Time

	started     chan struct{}
	terminal    chan struct{}
	nativeAck   chan struct{}
	processDone chan struct{}
}

type executeFrame struct {
	ProtocolVersion  int              `json:"protocol_version"`
	Type             string           `json:"type"`
	ExecutionID      ExecutionID      `json:"execution_id"`
	Kind             ExecutionKind    `json:"kind"`
	WorkingDirectory string           `json:"working_directory"`
	Prompt           string           `json:"prompt"`
	OutputSchema     json.RawMessage  `json:"output_schema,omitempty"`
	Sandbox          SandboxMode      `json:"sandbox"`
	AllowedTools     []ToolCapability `json:"allowed_tools"`
}

type cancelFrame struct {
	ProtocolVersion int         `json:"protocol_version"`
	Type            string      `json:"type"`
	ExecutionID     ExecutionID `json:"execution_id"`
}

type runtimeFrame struct {
	ProtocolVersion    int                `json:"protocol_version"`
	ExecutionID        ExecutionID        `json:"execution_id"`
	Sequence           uint64             `json:"sequence"`
	Type               string             `json:"type"`
	Text               string             `json:"text,omitempty"`
	RuntimeVersion     string             `json:"runtime_version,omitempty"`
	Status             ExecutionStatus    `json:"status,omitempty"`
	Result             json.RawMessage    `json:"result,omitempty"`
	ErrorCode          string             `json:"error_code,omitempty"`
	SafeMessage        string             `json:"safe_message,omitempty"`
	CancellationMethod CancellationMethod `json:"cancellation_method,omitempty"`
}

func (h *ProcessHost) CreateExecution(
	ctx context.Context,
	request ExecutionRequest,
) (ExecutionSnapshot, error) {
	profile, workingDirectory, err := h.validateExecutionRequest(request)
	if err != nil {
		return ExecutionSnapshot{}, err
	}
	if err := h.preflightCapabilities(request.RequiredCapabilities); err != nil {
		return ExecutionSnapshot{}, err
	}

	executionID, err := newExecutionID()
	if err != nil {
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime execution identity could not be created",
			true,
		)
	}

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host input could not be created",
			true,
		)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host output could not be created",
			true,
		)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host diagnostics could not be created",
			true,
		)
	}

	command := exec.Command(h.config.Command[0], h.config.Command[1:]...)
	command.Dir = workingDirectory
	command.Env = h.commandEnvironment()
	command.Stdin = stdinReader
	command.Stdout = stdoutWriter
	command.Stderr = stderrWriter
	configureProcessGroup(command)

	now := time.Now().UTC()
	execution := &managedExecution{
		snapshot: ExecutionSnapshot{
			ID:        executionID,
			Kind:      request.Kind,
			Status:    StatusStarting,
			CreatedAt: now,
		},
		notify:      make(chan struct{}),
		command:     command,
		stdin:       stdinWriter,
		started:     make(chan struct{}),
		terminal:    make(chan struct{}),
		nativeAck:   make(chan struct{}),
		processDone: make(chan struct{}),
	}

	if err := command.Start(); err != nil {
		closeProcessFiles(
			stdinReader,
			stdinWriter,
			stdoutReader,
			stdoutWriter,
			stderrReader,
			stderrWriter,
		)
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host could not start",
			true,
		)
	}
	execution.pid = command.Process.Pid
	_ = stdinReader.Close()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		_ = signalProcessGroup(execution.pid, syscall.SIGKILL)
		_ = command.Wait()
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
		return ExecutionSnapshot{}, newRuntimeError(
			ErrorHostClosed,
			"runtime host is closed",
			false,
		)
	}
	h.executions[executionID] = execution
	h.mu.Unlock()

	readerDone := make(chan struct{})
	go h.readFrames(execution, stdoutReader, readerDone)
	go func() {
		_, _ = io.Copy(io.Discard, stderrReader)
		_ = stderrReader.Close()
	}()
	go h.waitForProcess(execution, readerDone)

	frame := executeFrame{
		ProtocolVersion:  ProtocolVersion,
		Type:             "execute",
		ExecutionID:      executionID,
		Kind:             request.Kind,
		WorkingDirectory: workingDirectory,
		Prompt:           request.Prompt,
		OutputSchema:     cloneRawMessage(request.OutputSchema),
		Sandbox:          profile.Sandbox,
		AllowedTools:     append([]ToolCapability{}, profile.AllowedTools...),
	}
	timer := time.NewTimer(h.config.StartupTimeout)
	defer timer.Stop()
	writeResult := make(chan error, 1)
	go func() {
		writeResult <- h.writeFrame(execution, frame)
	}()
	select {
	case err := <-writeResult:
		if err == nil {
			break
		}
		h.failExecution(
			execution,
			ErrorRuntimeUnavailable,
			"runtime host could not accept the execution",
		)
		h.forceStop(execution, syscall.SIGKILL)
		return ExecutionSnapshot{}, h.snapshotError(execution)
	case <-ctx.Done():
		execution.mu.Lock()
		execution.cancelRequested = true
		execution.mu.Unlock()
		h.setCancellationMethod(execution, CancellationSIGKILL)
		h.forceStop(execution, syscall.SIGKILL)
		return ExecutionSnapshot{}, ctx.Err()
	case <-timer.C:
		h.failExecution(
			execution,
			ErrorRuntimeUnavailable,
			"runtime host preflight timed out",
		)
		h.forceStop(execution, syscall.SIGKILL)
		return ExecutionSnapshot{}, h.snapshotError(execution)
	}

	select {
	case <-execution.started:
		return h.snapshot(execution), nil
	case <-execution.terminal:
		return ExecutionSnapshot{}, h.snapshotError(execution)
	case <-ctx.Done():
		cancelCtx, cancel := context.WithTimeout(
			context.Background(),
			h.config.NativeCancelTimeout+h.config.TerminateTimeout+h.config.KillTimeout,
		)
		_, _ = h.CancelExecution(cancelCtx, executionID)
		cancel()
		return ExecutionSnapshot{}, ctx.Err()
	case <-timer.C:
		h.failExecution(
			execution,
			ErrorRuntimeUnavailable,
			"runtime host preflight timed out",
		)
		h.forceStop(execution, syscall.SIGKILL)
		return ExecutionSnapshot{}, h.snapshotError(execution)
	}
}

func (h *ProcessHost) SubscribeExecution(
	ctx context.Context,
	executionID ExecutionID,
) (<-chan Event, error) {
	execution, err := h.acquireSubscriber(executionID)
	if err != nil {
		return nil, err
	}
	output := make(chan Event, h.config.SubscriberBufferSize)
	go func() {
		defer close(output)
		expectsResultRead := false
		defer func() {
			h.releaseSubscriber(execution, expectsResultRead)
		}()
		index := 0
		for {
			execution.mu.Lock()
			pending := append([]Event(nil), execution.events[index:]...)
			index = len(execution.events)
			terminal := execution.snapshot.Status.Terminal()
			processClosed := execution.processClosed
			notify := execution.notify
			execution.mu.Unlock()

			for _, event := range pending {
				select {
				case output <- event:
				case <-ctx.Done():
					return
				}
			}
			if terminal && processClosed {
				expectsResultRead = true
				return
			}
			select {
			case <-notify:
			case <-execution.processDone:
			case <-ctx.Done():
				return
			}
		}
	}()
	return output, nil
}

func (h *ProcessHost) CancelExecution(
	ctx context.Context,
	executionID ExecutionID,
) (CancellationResult, error) {
	execution, err := h.execution(executionID)
	if err != nil {
		return CancellationResult{}, err
	}

	execution.mu.Lock()
	if execution.snapshot.Status.Terminal() {
		status := execution.snapshot.Status
		method := execution.snapshot.CancellationMethod
		if method == CancellationNone {
			method = CancellationAlreadyTerminal
		}
		execution.mu.Unlock()
		return CancellationResult{
			ExecutionID: executionID,
			Status:      status,
			Method:      method,
		}, nil
	}
	firstRequest := !execution.cancelRequested
	execution.cancelRequested = true
	execution.mu.Unlock()

	if firstRequest {
		if err := h.writeFrame(execution, cancelFrame{
			ProtocolVersion: ProtocolVersion,
			Type:            "cancel",
			ExecutionID:     executionID,
		}); err != nil {
			h.setCancellationMethod(execution, CancellationSIGTERM)
			h.forceStop(execution, syscall.SIGTERM)
		}
	}

	nativeTimer := time.NewTimer(h.config.NativeCancelTimeout)
	defer nativeTimer.Stop()
	select {
	case <-execution.nativeAck:
		h.setCancellationMethod(execution, CancellationNativeInterrupt)
	case <-execution.processDone:
		return h.cancellationResult(execution), nil
	case <-ctx.Done():
		return CancellationResult{}, ctx.Err()
	case <-nativeTimer.C:
		h.setCancellationMethod(execution, CancellationSIGTERM)
		h.forceStop(execution, syscall.SIGTERM)
	}

	if h.waitForDone(ctx, execution.processDone, h.config.TerminateTimeout) {
		return h.cancellationResult(execution), nil
	}

	h.setCancellationMethod(execution, CancellationSIGKILL)
	h.forceStop(execution, syscall.SIGKILL)
	if !h.waitForDone(ctx, execution.processDone, h.config.KillTimeout) {
		if err := ctx.Err(); err != nil {
			return CancellationResult{}, err
		}
		return CancellationResult{}, newRuntimeError(
			ErrorExecutionFailed,
			"runtime process could not be reaped",
			true,
		)
	}
	return h.cancellationResult(execution), nil
}

func (h *ProcessHost) GetExecution(
	ctx context.Context,
	executionID ExecutionID,
) (ExecutionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ExecutionSnapshot{}, err
	}
	execution, err := h.execution(executionID)
	if err != nil {
		return ExecutionSnapshot{}, err
	}
	snapshot := h.snapshotAndMarkRead(execution)
	h.evictTerminalExecutions()
	return snapshot, nil
}

func (h *ProcessHost) Close(ctx context.Context) error {
	h.mu.Lock()
	h.closed = true
	executions := make([]*managedExecution, 0, len(h.executions))
	for _, execution := range h.executions {
		executions = append(executions, execution)
	}
	h.mu.Unlock()

	var closeErrors []error
	for _, execution := range executions {
		execution.mu.Lock()
		terminal := execution.snapshot.Status.Terminal()
		executionID := execution.snapshot.ID
		execution.mu.Unlock()
		if !terminal {
			if _, err := h.CancelExecution(ctx, executionID); err != nil {
				closeErrors = append(closeErrors, err)
				h.forceStopForClose(execution)
			}
		}
		select {
		case <-execution.processDone:
		case <-ctx.Done():
			for _, pending := range executions {
				h.forceStopForClose(pending)
			}
			closeErrors = append(closeErrors, ctx.Err())
			return errors.Join(closeErrors...)
		}
	}
	return errors.Join(closeErrors...)
}

func (h *ProcessHost) Diagnostics() ProcessDiagnostics {
	h.mu.RLock()
	executions := make([]*managedExecution, 0, len(h.executions))
	for _, execution := range h.executions {
		executions = append(executions, execution)
	}
	h.mu.RUnlock()

	diagnostics := ProcessDiagnostics{TrackedExecutions: len(executions)}
	for _, execution := range executions {
		execution.mu.Lock()
		processClosed := execution.processClosed
		pid := execution.pid
		execution.mu.Unlock()
		if !processClosed {
			diagnostics.ActiveExecutions++
		}
		if processGroupAlive(pid) {
			diagnostics.LiveProcessGroups++
		}
	}
	return diagnostics
}

func (h *ProcessHost) validateExecutionRequest(
	request ExecutionRequest,
) (Profile, string, error) {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return Profile{}, "", newRuntimeError(
			ErrorHostClosed,
			"runtime host is closed",
			false,
		)
	}
	profile, exists := h.config.Profiles[request.Kind]
	if !exists {
		return Profile{}, "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime execution kind is not allowed",
			false,
		)
	}
	if strings.TrimSpace(request.Prompt) == "" ||
		len(request.Prompt) > h.config.MaxPromptBytes {
		return Profile{}, "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime prompt is missing or exceeds the allowed size",
			false,
		)
	}
	if len(request.OutputSchema) > h.config.MaxSchemaBytes ||
		(len(request.OutputSchema) > 0 && !jsonObject(request.OutputSchema)) {
		return Profile{}, "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime output schema is invalid",
			false,
		)
	}
	if request.Kind != ExecutionKindAssistant && len(request.OutputSchema) == 0 {
		return Profile{}, "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime output schema is required for this execution kind",
			false,
		)
	}
	workingDirectory, err := h.resolveWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return Profile{}, "", err
	}
	return profile, workingDirectory, nil
}

func (h *ProcessHost) resolveWorkingDirectory(value string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime working directory must be absolute",
			false,
		)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(value))
	if err != nil {
		return "", newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime working directory is unavailable",
			false,
		)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime working directory is unavailable",
			false,
		)
	}
	relative, err := filepath.Rel(h.config.WorkRoot, resolved)
	if err != nil ||
		relative == "." ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", newRuntimeError(
			ErrorInvalidRequest,
			"runtime working directory is outside the managed root",
			false,
		)
	}
	return resolved, nil
}

func (h *ProcessHost) preflightCapabilities(required []string) error {
	seen := make(map[string]struct{}, len(required))
	for _, name := range required {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
			return newRuntimeError(
				ErrorInvalidRequest,
				"runtime capability name is invalid",
				false,
			)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		candidate, allowed := h.config.Capabilities[name]
		if !allowed {
			return newRuntimeError(
				ErrorRuntimeUnavailable,
				"required runtime capability is not configured",
				false,
			)
		}
		if filepath.IsAbs(candidate) {
			info, err := os.Stat(candidate)
			if err != nil || info.IsDir() {
				return newRuntimeError(
					ErrorRuntimeUnavailable,
					"required runtime capability is unavailable",
					false,
				)
			}
			continue
		}
		if _, err := exec.LookPath(candidate); err != nil {
			return newRuntimeError(
				ErrorRuntimeUnavailable,
				"required runtime capability is unavailable",
				false,
			)
		}
	}
	return nil
}

func (h *ProcessHost) readFrames(
	execution *managedExecution,
	reader *os.File,
	done chan<- struct{},
) {
	defer close(done)
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), h.config.MaxFrameBytes)
	for scanner.Scan() {
		frame, err := decodeRuntimeFrame(scanner.Bytes())
		if err != nil {
			h.failExecution(
				execution,
				ErrorProtocol,
				"runtime host emitted an invalid protocol frame",
			)
			h.forceStop(execution, syscall.SIGKILL)
			return
		}
		if !h.acceptFrame(execution, frame) {
			h.forceStop(execution, syscall.SIGKILL)
			return
		}
	}
	if scanner.Err() != nil {
		h.failExecution(
			execution,
			ErrorProtocol,
			"runtime host event stream exceeded protocol limits",
		)
		h.forceStop(execution, syscall.SIGKILL)
	}
}

func (h *ProcessHost) acceptFrame(
	execution *managedExecution,
	frame runtimeFrame,
) bool {
	execution.mu.Lock()
	defer execution.mu.Unlock()

	if frame.ProtocolVersion != ProtocolVersion ||
		frame.ExecutionID == "" ||
		frame.ExecutionID != execution.snapshot.ID ||
		frame.Sequence != execution.lastSequence+1 ||
		execution.terminalFrame {
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime event identity or ordering is invalid",
		)
		return false
	}
	execution.lastSequence = frame.Sequence
	now := time.Now().UTC()

	switch frame.Type {
	case "ready":
		if execution.snapshot.Status != StatusStarting ||
			strings.TrimSpace(frame.RuntimeVersion) == "" {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime readiness frame is invalid",
			)
			return false
		}
		execution.snapshot.Status = StatusRunning
		execution.snapshot.RuntimeVersion = frame.RuntimeVersion
		execution.snapshot.StartedAt = &now
		if !h.appendEventLocked(execution, Event{
			ExecutionID: execution.snapshot.ID,
			Sequence:    frame.Sequence,
			Type:        EventStarted,
			ObservedAt:  now,
		}) {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime event stream exceeded configured limits",
			)
			return false
		}
		if !execution.startedClosed {
			close(execution.started)
			execution.startedClosed = true
		}
	case "output_delta":
		if execution.snapshot.Status != StatusRunning || frame.Text == "" {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime output event is invalid",
			)
			return false
		}
		if !h.appendEventLocked(execution, Event{
			ExecutionID: execution.snapshot.ID,
			Sequence:    frame.Sequence,
			Type:        EventOutputDelta,
			Text:        frame.Text,
			ObservedAt:  now,
		}) {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime event stream exceeded configured limits",
			)
			return false
		}
	case "progress":
		if execution.snapshot.Status != StatusRunning ||
			!h.appendEventLocked(execution, Event{
				ExecutionID: execution.snapshot.ID,
				Sequence:    frame.Sequence,
				Type:        EventProgress,
				Text:        frame.Text,
				ObservedAt:  now,
			}) {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime progress event is invalid",
			)
			return false
		}
	case "cancel_ack":
		if !execution.cancelRequested ||
			frame.CancellationMethod != CancellationNativeInterrupt {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime cancellation acknowledgement is invalid",
			)
			return false
		}
		if execution.snapshot.CancellationMethod == CancellationNone {
			execution.snapshot.CancellationMethod = CancellationNativeInterrupt
		}
		if !h.appendEventLocked(execution, Event{
			ExecutionID: execution.snapshot.ID,
			Sequence:    frame.Sequence,
			Type:        EventCancellationRequest,
			ObservedAt:  now,
		}) {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime event stream exceeded configured limits",
			)
			return false
		}
		if !execution.nativeAckClosed {
			close(execution.nativeAck)
			execution.nativeAckClosed = true
		}
	case "terminal":
		if !h.acceptTerminalLocked(execution, frame, now) {
			return false
		}
	default:
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime event type is not supported",
		)
		return false
	}
	return true
}

func (h *ProcessHost) acceptTerminalLocked(
	execution *managedExecution,
	frame runtimeFrame,
	now time.Time,
) bool {
	switch frame.Status {
	case StatusCompleted:
		if execution.snapshot.Status != StatusRunning ||
			len(frame.Result) == 0 ||
			len(frame.Result) > h.config.MaxResultBytes ||
			!jsonObject(frame.Result) {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime completed with an invalid structured result",
			)
			return false
		}
		execution.snapshot.Result = cloneRawMessage(frame.Result)
	case StatusFailed:
		if strings.TrimSpace(frame.ErrorCode) == "" ||
			strings.TrimSpace(frame.SafeMessage) == "" {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime failed without a safe error",
			)
			return false
		}
		execution.snapshot.ErrorCode = frame.ErrorCode
		execution.snapshot.SafeMessage = frame.SafeMessage
	case StatusCancelled:
		if !execution.cancelRequested {
			h.failExecutionLocked(
				execution,
				ErrorProtocol,
				"runtime cancelled without a matching request",
			)
			return false
		}
	default:
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime terminal status is invalid",
		)
		return false
	}

	execution.snapshot.Status = frame.Status
	execution.snapshot.CompletedAt = &now
	execution.terminalFrame = true
	if !h.appendEventLocked(execution, Event{
		ExecutionID: execution.snapshot.ID,
		Sequence:    frame.Sequence,
		Type:        EventTerminal,
		ObservedAt:  now,
	}) {
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime event stream exceeded configured limits",
		)
		return false
	}
	if !execution.terminalClosed {
		close(execution.terminal)
		execution.terminalClosed = true
	}
	go h.ensureTerminalProcessExit(execution)
	return true
}

func (h *ProcessHost) appendEventLocked(
	execution *managedExecution,
	event Event,
) bool {
	eventBytes := len(event.Text) + 64
	if len(execution.events)+1 > h.config.MaxEvents ||
		execution.bytes+eventBytes > h.config.MaxEventBytes {
		return false
	}
	execution.events = append(execution.events, event)
	execution.bytes += eventBytes
	close(execution.notify)
	execution.notify = make(chan struct{})
	return true
}

func (h *ProcessHost) failExecution(
	execution *managedExecution,
	code string,
	message string,
) {
	execution.mu.Lock()
	h.failExecutionLocked(execution, code, message)
	execution.mu.Unlock()
}

func (h *ProcessHost) failExecutionLocked(
	execution *managedExecution,
	code string,
	message string,
) {
	now := time.Now().UTC()
	execution.lastSequence++
	execution.snapshot.Status = StatusFailed
	execution.snapshot.Result = nil
	execution.snapshot.ErrorCode = code
	execution.snapshot.SafeMessage = message
	execution.snapshot.CompletedAt = &now
	execution.terminalFrame = true
	event := Event{
		ExecutionID: execution.snapshot.ID,
		Sequence:    execution.lastSequence,
		Type:        EventTerminal,
		ObservedAt:  now,
	}
	execution.events = append(execution.events, event)
	execution.bytes += 64
	close(execution.notify)
	execution.notify = make(chan struct{})
	if !execution.terminalClosed {
		close(execution.terminal)
		execution.terminalClosed = true
	}
}

func (h *ProcessHost) waitForProcess(
	execution *managedExecution,
	readerDone <-chan struct{},
) {
	waitErr := execution.command.Wait()
	cleanupSignal := h.ensureProcessGroupStopped(execution.pid)
	_ = execution.stdin.Close()
	<-readerDone

	execution.mu.Lock()
	if cleanupSignal != CancellationNone &&
		execution.snapshot.Status == StatusCompleted {
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime child process required orphan cleanup",
		)
	}
	if !execution.snapshot.Status.Terminal() {
		now := time.Now().UTC()
		execution.lastSequence++
		if execution.cancelRequested {
			execution.snapshot.Status = StatusCancelled
			if execution.snapshot.CancellationMethod == CancellationNone {
				execution.snapshot.CancellationMethod = cleanupSignal
			}
		} else {
			execution.snapshot.Status = StatusFailed
			execution.snapshot.ErrorCode = ErrorProtocol
			execution.snapshot.SafeMessage = "runtime host exited without a terminal result"
		}
		execution.snapshot.CompletedAt = &now
		execution.events = append(execution.events, Event{
			ExecutionID: execution.snapshot.ID,
			Sequence:    execution.lastSequence,
			Type:        EventTerminal,
			ObservedAt:  now,
		})
		close(execution.notify)
		execution.notify = make(chan struct{})
		if !execution.terminalClosed {
			close(execution.terminal)
			execution.terminalClosed = true
		}
	} else if waitErr != nil &&
		execution.snapshot.Status == StatusCompleted {
		h.failExecutionLocked(
			execution,
			ErrorProtocol,
			"runtime host exited abnormally after completion",
		)
	}
	execution.processClosed = true
	close(execution.processDone)
	execution.mu.Unlock()
	h.evictTerminalExecutions()
}

func (h *ProcessHost) ensureProcessGroupStopped(pid int) CancellationMethod {
	deadline := time.Now().Add(h.config.PostExitGrace)
	for processGroupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !processGroupAlive(pid) {
		return CancellationNone
	}
	_ = signalProcessGroup(pid, syscall.SIGTERM)
	deadline = time.Now().Add(h.config.TerminateTimeout)
	for processGroupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !processGroupAlive(pid) {
		return CancellationSIGTERM
	}
	_ = signalProcessGroup(pid, syscall.SIGKILL)
	deadline = time.Now().Add(h.config.KillTimeout)
	for processGroupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	return CancellationSIGKILL
}

func (h *ProcessHost) forceStop(
	execution *managedExecution,
	signal syscall.Signal,
) {
	_ = signalProcessGroup(execution.pid, signal)
}

func (h *ProcessHost) forceStopForClose(execution *managedExecution) {
	execution.mu.Lock()
	if execution.processClosed {
		execution.mu.Unlock()
		return
	}
	execution.cancelRequested = true
	execution.mu.Unlock()
	h.setCancellationMethod(execution, CancellationSIGKILL)
	h.forceStop(execution, syscall.SIGKILL)
}

func (h *ProcessHost) ensureTerminalProcessExit(execution *managedExecution) {
	if h.waitForDone(
		context.Background(),
		execution.processDone,
		h.config.TerminateTimeout,
	) {
		return
	}
	h.forceStop(execution, syscall.SIGTERM)
	if h.waitForDone(
		context.Background(),
		execution.processDone,
		h.config.TerminateTimeout,
	) {
		return
	}
	h.forceStop(execution, syscall.SIGKILL)
}

func (h *ProcessHost) setCancellationMethod(
	execution *managedExecution,
	method CancellationMethod,
) {
	execution.mu.Lock()
	switch {
	case method == CancellationSIGKILL:
		execution.snapshot.CancellationMethod = method
	case method == CancellationSIGTERM &&
		execution.snapshot.CancellationMethod != CancellationSIGKILL:
		execution.snapshot.CancellationMethod = method
	case execution.snapshot.CancellationMethod == CancellationNone:
		execution.snapshot.CancellationMethod = method
	}
	execution.mu.Unlock()
}

func (h *ProcessHost) waitForDone(
	ctx context.Context,
	done <-chan struct{},
	timeout time.Duration,
) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func (h *ProcessHost) cancellationResult(
	execution *managedExecution,
) CancellationResult {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return CancellationResult{
		ExecutionID: execution.snapshot.ID,
		Status:      execution.snapshot.Status,
		Method:      execution.snapshot.CancellationMethod,
	}
}

func (h *ProcessHost) execution(
	executionID ExecutionID,
) (*managedExecution, error) {
	if executionID == "" {
		return nil, newRuntimeError(
			ErrorExecutionNotFound,
			"runtime execution was not found",
			false,
		)
	}
	h.mu.RLock()
	execution := h.executions[executionID]
	h.mu.RUnlock()
	if execution == nil {
		return nil, newRuntimeError(
			ErrorExecutionNotFound,
			"runtime execution was not found",
			false,
		)
	}
	return execution, nil
}

func (h *ProcessHost) acquireSubscriber(
	executionID ExecutionID,
) (*managedExecution, error) {
	if executionID == "" {
		return nil, newRuntimeError(
			ErrorExecutionNotFound,
			"runtime execution was not found",
			false,
		)
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	execution := h.executions[executionID]
	if execution == nil {
		return nil, newRuntimeError(
			ErrorExecutionNotFound,
			"runtime execution was not found",
			false,
		)
	}
	execution.mu.Lock()
	execution.subscribers++
	execution.mu.Unlock()
	return execution, nil
}

func (h *ProcessHost) releaseSubscriber(
	execution *managedExecution,
	expectsResultRead bool,
) {
	execution.mu.Lock()
	if execution.subscribers > 0 {
		execution.subscribers--
	}
	if expectsResultRead {
		execution.pendingReads++
		execution.retainUntil = time.Now().UTC().Add(h.config.ResultReadGrace)
	}
	execution.mu.Unlock()
	if expectsResultRead {
		time.AfterFunc(h.config.ResultReadGrace, h.evictTerminalExecutions)
	}
	h.evictTerminalExecutions()
}

func (h *ProcessHost) evictTerminalExecutions() {
	type candidate struct {
		id          ExecutionID
		completedAt time.Time
		createdAt   time.Time
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	candidates := make([]candidate, 0, len(h.executions))
	for executionID, execution := range h.executions {
		execution.mu.Lock()
		terminal := execution.snapshot.Status.Terminal()
		processClosed := execution.processClosed
		createdAt := execution.snapshot.CreatedAt
		completedAt := execution.snapshot.CompletedAt
		subscribers := execution.subscribers
		pendingReads := execution.pendingReads
		retainUntil := execution.retainUntil
		execution.mu.Unlock()
		if !terminal ||
			!processClosed ||
			completedAt == nil ||
			subscribers > 0 ||
			(pendingReads > 0 && now.Before(retainUntil)) {
			continue
		}
		candidates = append(candidates, candidate{
			id:          executionID,
			completedAt: *completedAt,
			createdAt:   createdAt,
		})
	}
	if len(candidates) <= h.config.MaxRetainedExecutions {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].createdAt.Before(candidates[j].createdAt)
		}
		if !candidates[i].completedAt.Equal(candidates[j].completedAt) {
			return candidates[i].completedAt.Before(candidates[j].completedAt)
		}
		return candidates[i].id < candidates[j].id
	})
	for _, expired := range candidates[:len(candidates)-h.config.MaxRetainedExecutions] {
		delete(h.executions, expired.id)
	}
}

func (h *ProcessHost) snapshot(
	execution *managedExecution,
) ExecutionSnapshot {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	snapshot := execution.snapshot
	snapshot.Result = cloneRawMessage(snapshot.Result)
	return snapshot
}

func (h *ProcessHost) snapshotAndMarkRead(
	execution *managedExecution,
) ExecutionSnapshot {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	snapshot := execution.snapshot
	snapshot.Result = cloneRawMessage(snapshot.Result)
	if snapshot.Status.Terminal() && execution.pendingReads > 0 {
		execution.pendingReads--
	}
	if execution.pendingReads == 0 {
		execution.retainUntil = time.Time{}
	}
	return snapshot
}

func (h *ProcessHost) snapshotError(
	execution *managedExecution,
) error {
	snapshot := h.snapshot(execution)
	code := snapshot.ErrorCode
	if code == "" {
		code = ErrorExecutionFailed
	}
	message := snapshot.SafeMessage
	if message == "" {
		message = "runtime execution failed"
	}
	return newRuntimeError(
		code,
		message,
		code == ErrorRuntimeUnavailable || code == ErrorExecutionFailed,
	)
}

func (h *ProcessHost) writeFrame(
	execution *managedExecution,
	frame any,
) error {
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(payload)+1 > h.config.MaxFrameBytes {
		return fmt.Errorf("runtime command exceeds frame limit")
	}
	execution.writeMu.Lock()
	defer execution.writeMu.Unlock()
	if _, err := execution.stdin.Write(append(payload, '\n')); err != nil {
		return err
	}
	return nil
}

func (h *ProcessHost) commandEnvironment() []string {
	if h.config.Environment != nil {
		return environmentMap(h.config.Environment)
	}
	allowed := []string{
		"CODEX_HOME",
		"HOME",
		"LANG",
		"LC_ALL",
		"PATH",
		"SSL_CERT_DIR",
		"SSL_CERT_FILE",
		"TMPDIR",
		"XDG_CONFIG_HOME",
	}
	environment := make(map[string]string, len(allowed))
	for _, name := range allowed {
		if value, exists := os.LookupEnv(name); exists {
			environment[name] = value
		}
	}
	return environmentMap(environment)
}

func normalizeProcessHostConfig(
	config ProcessHostConfig,
) (ProcessHostConfig, error) {
	if len(config.Command) == 0 || strings.TrimSpace(config.Command[0]) == "" {
		return ProcessHostConfig{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host command is not configured",
			false,
		)
	}
	commandPath, err := exec.LookPath(config.Command[0])
	if err != nil {
		return ProcessHostConfig{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime host command is unavailable",
			false,
		)
	}
	config.Command = append([]string(nil), config.Command...)
	config.Command[0] = commandPath

	if !filepath.IsAbs(config.WorkRoot) {
		return ProcessHostConfig{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime managed root must be absolute",
			false,
		)
	}
	workRoot, err := filepath.EvalSymlinks(filepath.Clean(config.WorkRoot))
	if err != nil {
		return ProcessHostConfig{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime managed root is unavailable",
			false,
		)
	}
	info, err := os.Stat(workRoot)
	if err != nil || !info.IsDir() {
		return ProcessHostConfig{}, newRuntimeError(
			ErrorRuntimeUnavailable,
			"runtime managed root is unavailable",
			false,
		)
	}
	config.WorkRoot = workRoot

	if len(config.Profiles) == 0 {
		config.Profiles = DefaultProfiles()
	} else {
		config.Profiles = cloneProfiles(config.Profiles)
	}
	for kind, profile := range config.Profiles {
		if strings.TrimSpace(string(kind)) == "" {
			return ProcessHostConfig{}, newRuntimeError(
				ErrorInvalidRequest,
				"runtime profile kind is invalid",
				false,
			)
		}
		switch profile.Sandbox {
		case SandboxReadOnly, SandboxWorkspaceWrite:
		default:
			return ProcessHostConfig{}, newRuntimeError(
				ErrorInvalidRequest,
				"runtime sandbox profile is invalid",
				false,
			)
		}
		seenTools := make(map[ToolCapability]struct{}, len(profile.AllowedTools))
		for _, tool := range profile.AllowedTools {
			if tool != ToolWebSearch {
				return ProcessHostConfig{}, newRuntimeError(
					ErrorInvalidRequest,
					"runtime tool profile contains an unknown capability",
					false,
				)
			}
			if _, exists := seenTools[tool]; exists {
				return ProcessHostConfig{}, newRuntimeError(
					ErrorInvalidRequest,
					"runtime tool profile contains a duplicate capability",
					false,
				)
			}
			seenTools[tool] = struct{}{}
		}
	}
	config.Capabilities = cloneStringMap(config.Capabilities)
	config.Environment = cloneOptionalStringMap(config.Environment)

	if config.StartupTimeout <= 0 {
		config.StartupTimeout = defaultStartupTimeout
	}
	if config.NativeCancelTimeout <= 0 {
		config.NativeCancelTimeout = defaultNativeCancelWait
	}
	if config.TerminateTimeout <= 0 {
		config.TerminateTimeout = defaultTerminateWait
	}
	if config.KillTimeout <= 0 {
		config.KillTimeout = defaultKillWait
	}
	if config.PostExitGrace <= 0 {
		config.PostExitGrace = defaultPostExitGrace
	}
	if config.MaxFrameBytes <= 0 {
		config.MaxFrameBytes = defaultMaxFrameBytes
	}
	if config.MaxEventBytes <= 0 {
		config.MaxEventBytes = defaultMaxEventBytes
	}
	if config.MaxEvents <= 0 {
		config.MaxEvents = defaultMaxEvents
	}
	if config.MaxPromptBytes <= 0 {
		config.MaxPromptBytes = defaultMaxPromptBytes
	}
	if config.MaxSchemaBytes <= 0 {
		config.MaxSchemaBytes = defaultMaxSchemaBytes
	}
	if config.MaxResultBytes <= 0 {
		config.MaxResultBytes = defaultMaxResultBytes
	}
	if config.SubscriberBufferSize <= 0 {
		config.SubscriberBufferSize = defaultSubscriberBuffer
	}
	if config.MaxRetainedExecutions <= 0 {
		config.MaxRetainedExecutions = defaultMaxRetained
	}
	if config.ResultReadGrace <= 0 {
		config.ResultReadGrace = defaultResultReadGrace
	}
	return config, nil
}

func decodeRuntimeFrame(payload []byte) (runtimeFrame, error) {
	var frame runtimeFrame
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&frame); err != nil {
		return runtimeFrame{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return runtimeFrame{}, fmt.Errorf("runtime frame contains trailing data")
	}
	return frame, nil
}

func jsonObject(payload json.RawMessage) bool {
	if !json.Valid(payload) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(payload, &object) == nil && object != nil
}

func newExecutionID() (ExecutionID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return ExecutionID(encoded), nil
}

func environmentMap(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	output := make([]string, 0, len(keys))
	for _, key := range keys {
		output = append(output, key+"="+input[key])
	}
	return output
}

func cloneProfiles(input map[ExecutionKind]Profile) map[ExecutionKind]Profile {
	output := make(map[ExecutionKind]Profile, len(input))
	for kind, profile := range input {
		profile.AllowedTools = append(
			[]ToolCapability(nil),
			profile.AllowedTools...,
		)
		output[kind] = profile
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneOptionalStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	return cloneStringMap(input)
}

func cloneRawMessage(input json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), input...)
}

func closeProcessFiles(files ...*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}
