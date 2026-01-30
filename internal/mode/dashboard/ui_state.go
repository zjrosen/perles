package dashboard

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// maxCommandLogEntries is the maximum number of entries to keep in the command log.
// When exceeded, oldest entries are removed (FIFO eviction).
const maxCommandLogEntries = 2000

// CommandLogEntry represents a single command in the log.
// This is used for debug mode command log display.
type CommandLogEntry struct {
	Timestamp   time.Time
	CommandType command.CommandType
	CommandID   string
	Source      command.CommandSource
	Success     bool
	Error       string // Converted from error for display
	Duration    time.Duration
	TraceID     string // Distributed trace ID for correlation (empty if tracing disabled)
}

// maxCachedWorkflows is the maximum number of workflow UI states to keep in cache.
// When exceeded, the oldest non-running, non-selected workflow state is evicted.
const maxCachedWorkflows = 10

// maxFabricEvents is the maximum number of fabric events to keep per workflow.
// This cap prevents unbounded memory growth in long-running sessions.
// When exceeded, oldest events are removed (FIFO eviction).
const maxFabricEvents = 500

// WorkflowUIState holds accumulated UI state for a single workflow.
// This state persists across workflow switches in the dashboard.
type WorkflowUIState struct {
	// Coordinator pane state
	CoordinatorMessages   []chatrender.Message
	CoordinatorMetrics    *metrics.TokenMetrics
	CoordinatorStatus     events.ProcessStatus
	CoordinatorQueueCount int

	// Message pane state (filtered to message.posted and reply.posted events only)
	FabricEvents []fabric.Event

	// Worker pane state
	WorkerIDs         []string
	WorkerStatus      map[string]events.ProcessStatus
	WorkerPhases      map[string]events.ProcessPhase
	WorkerMessages    map[string][]chatrender.Message
	WorkerMetrics     map[string]*metrics.TokenMetrics
	WorkerQueueCounts map[string]int

	// Scroll position persistence (integer offsets for VirtualSelectablePane)
	// These store scroll offsets to preserve scroll positions across workflow switches.
	CoordinatorScrollOffset int
	WorkerScrollOffsets     map[string]int

	// Notification state
	// HasNotification is set to true when a ProcessUserNotification event is received.
	// This is used to highlight the workflow row in the dashboard to draw user attention.
	// Cleared when the user selects the row and presses Enter.
	HasNotification bool

	// Epic tree state
	// These fields store minimal tree navigation state (enums and ID string)
	// to avoid memory pressure while still preserving user context.
	TreeDirection  tree.Direction
	TreeMode       tree.TreeMode
	TreeSelectedID string

	// Command log state (for debug mode)
	CommandLogEntries []CommandLogEntry

	// Cache metadata
	LastUpdated time.Time
}

// NewWorkflowUIState creates an empty WorkflowUIState with all maps initialized.
func NewWorkflowUIState() *WorkflowUIState {
	return &WorkflowUIState{
		CoordinatorMessages:     make([]chatrender.Message, 0),
		FabricEvents:            make([]fabric.Event, 0),
		WorkerIDs:               make([]string, 0),
		WorkerStatus:            make(map[string]events.ProcessStatus),
		WorkerPhases:            make(map[string]events.ProcessPhase),
		WorkerMessages:          make(map[string][]chatrender.Message),
		WorkerMetrics:           make(map[string]*metrics.TokenMetrics),
		WorkerQueueCounts:       make(map[string]int),
		CoordinatorScrollOffset: 0,
		WorkerScrollOffsets:     make(map[string]int),
		CommandLogEntries:       make([]CommandLogEntry, 0),
		LastUpdated:             time.Time{},
	}
}

// IsEmpty returns true if the state has no content.
func (s *WorkflowUIState) IsEmpty() bool {
	return len(s.CoordinatorMessages) == 0 &&
		len(s.FabricEvents) == 0 &&
		len(s.WorkerIDs) == 0
}
