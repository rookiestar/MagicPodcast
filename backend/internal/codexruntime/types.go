package codexruntime

import (
	"context"
	"encoding/json"
	"time"
)

const ProtocolVersion = 1

type ExecutionID string

type ExecutionKind string

const (
	ExecutionKindEpisodeNotes ExecutionKind = "episode_notes"
	ExecutionKindAssistant    ExecutionKind = "assistant"
	ExecutionKindSmoke        ExecutionKind = "smoke"
)

type SandboxMode string

const (
	SandboxReadOnly       SandboxMode = "read_only"
	SandboxWorkspaceWrite SandboxMode = "workspace_write"
)

type ToolCapability string

const (
	ToolWebSearch ToolCapability = "web_search"
)

type Profile struct {
	Sandbox      SandboxMode
	AllowedTools []ToolCapability
}

// ToolRestriction can only reduce the tools granted by an execution kind's
// host profile. A nil restriction uses the profile unchanged; an explicit
// empty Allowed list disables every tool for that execution.
type ToolRestriction struct {
	Allowed []ToolCapability
}

type ExecutionRequest struct {
	Kind                 ExecutionKind
	WorkingDirectory     string
	Prompt               string
	OutputSchema         json.RawMessage
	RequiredCapabilities []string
	ToolRestriction      *ToolRestriction
}

type ExecutionStatus string

const (
	StatusStarting  ExecutionStatus = "starting"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
)

func (s ExecutionStatus) Terminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

type EventType string

const (
	EventStarted             EventType = "started"
	EventOutputDelta         EventType = "output_delta"
	EventProgress            EventType = "progress"
	EventCancellationRequest EventType = "cancellation_requested"
	EventTerminal            EventType = "terminal"
)

type Event struct {
	ExecutionID ExecutionID `json:"execution_id"`
	Sequence    uint64      `json:"sequence"`
	Type        EventType   `json:"type"`
	Text        string      `json:"text,omitempty"`
	ObservedAt  time.Time   `json:"observed_at"`
}

type CancellationMethod string

const (
	CancellationNone            CancellationMethod = ""
	CancellationNativeInterrupt CancellationMethod = "native_interrupt"
	CancellationSIGTERM         CancellationMethod = "sigterm"
	CancellationSIGKILL         CancellationMethod = "sigkill"
	CancellationAlreadyTerminal CancellationMethod = "already_terminal"
)

type ExecutionSnapshot struct {
	ID                 ExecutionID        `json:"id"`
	Kind               ExecutionKind      `json:"kind"`
	Status             ExecutionStatus    `json:"status"`
	RuntimeVersion     string             `json:"runtime_version,omitempty"`
	Result             json.RawMessage    `json:"result,omitempty"`
	ErrorCode          string             `json:"error_code,omitempty"`
	SafeMessage        string             `json:"safe_message,omitempty"`
	CancellationMethod CancellationMethod `json:"cancellation_method,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
	StartedAt          *time.Time         `json:"started_at,omitempty"`
	CompletedAt        *time.Time         `json:"completed_at,omitempty"`
}

type CancellationResult struct {
	ExecutionID ExecutionID        `json:"execution_id"`
	Status      ExecutionStatus    `json:"status"`
	Method      CancellationMethod `json:"method"`
}

// Runtime is the provider-neutral seam used by processing and interactive
// callers. Implementations own process supervision, protocol validation,
// cancellation fallback, and cleanup behind this interface.
type Runtime interface {
	CreateExecution(context.Context, ExecutionRequest) (ExecutionSnapshot, error)
	SubscribeExecution(context.Context, ExecutionID) (<-chan Event, error)
	CancelExecution(context.Context, ExecutionID) (CancellationResult, error)
	GetExecution(context.Context, ExecutionID) (ExecutionSnapshot, error)
	Close(context.Context) error
}
