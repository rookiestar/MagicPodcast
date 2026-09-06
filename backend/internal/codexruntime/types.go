package codexruntime

import (
	"context"
	"encoding/json"
	"time"
)

// ProtocolVersion is the stdio JSONL contract between the Go parent and the
// Python host. Version 2 carried the resolved model profile on execute
// frames. Version 3 adds structured provider-neutral progress frames; hosts
// and parents of different versions must fail explicitly instead of guessing
// model configuration or silently dropping activity payloads.
const ProtocolVersion = 3

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
	// ModelProfile optionally pins a trusted model configuration by stable
	// catalog ID. Empty keeps the runtime default configuration.
	ModelProfile ModelProfileID
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

// ProgressCategory is the provider-neutral class of one runtime activity.
// Values are part of the protocol contract; hosts must not invent others.
type ProgressCategory string

const (
	CategoryWebSearch   ProgressCategory = "web_search"
	CategoryReasoning   ProgressCategory = "reasoning"
	CategoryPlan        ProgressCategory = "plan"
	CategoryAgentMsg    ProgressCategory = "agent_message"
	CategoryTurn        ProgressCategory = "turn"
	CategoryGenericItem ProgressCategory = "item"
)

// ProgressState is the lifecycle state of one activity. An activity moves
// from started through zero or more updates into completed or failed, and is
// closed afterwards.
type ProgressState string

const (
	ProgressStarted   ProgressState = "started"
	ProgressUpdated   ProgressState = "updated"
	ProgressCompleted ProgressState = "completed"
	ProgressFailed    ProgressState = "failed"
)

// Progress is the sanitized, provider-neutral activity payload carried by
// EventProgress events. Display text is bounded plain text; metadata is a
// bounded set of public string facts (for example candidate domains). Raw
// prompts, reasoning content, private notes, managed paths, credentials, and
// provider payloads never reach this structure.
type Progress struct {
	ActivityID  string            `json:"activity_id"`
	Ordinal     uint64            `json:"ordinal"`
	Category    ProgressCategory  `json:"category"`
	State       ProgressState     `json:"state"`
	DisplayText string            `json:"display_text,omitempty"`
	ElapsedMS   int64             `json:"elapsed_ms,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Event struct {
	ExecutionID ExecutionID `json:"execution_id"`
	Sequence    uint64      `json:"sequence"`
	Type        EventType   `json:"type"`
	Text        string      `json:"text,omitempty"`
	Progress    *Progress   `json:"progress,omitempty"`
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
