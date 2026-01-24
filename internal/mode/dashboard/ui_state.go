package dashboard

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/tree"
)

// maxCachedWorkflows is the maximum number of workflow UI states to keep in cache.
// When exceeded, the oldest non-running, non-selected workflow state is evicted.
const maxCachedWorkflows = 10

// WorkflowUIState holds accumulated UI state for a single workflow.
// This state persists across workflow switches in the dashboard.
type WorkflowUIState struct {
	// Coordinator pane state
	CoordinatorMessages   []chatrender.Message
	CoordinatorMetrics    *metrics.TokenMetrics
	CoordinatorStatus     events.ProcessStatus
	CoordinatorQueueCount int

	// Message pane state
	MessageEntries []message.Entry

	// Worker pane state
	WorkerIDs         []string
	WorkerStatus      map[string]events.ProcessStatus
	WorkerPhases      map[string]events.ProcessPhase
	WorkerMessages    map[string][]chatrender.Message
	WorkerMetrics     map[string]*metrics.TokenMetrics
	WorkerQueueCounts map[string]int

	// Viewport state (scroll positions as percentages)
	CoordinatorScrollPercent float64
	MessageScrollPercent     float64
	WorkerScrollPercents     map[string]float64

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

	// Cache metadata
	LastUpdated time.Time
}

// NewWorkflowUIState creates an empty WorkflowUIState with all maps initialized.
func NewWorkflowUIState() *WorkflowUIState {
	return &WorkflowUIState{
		CoordinatorMessages:      make([]chatrender.Message, 0),
		MessageEntries:           make([]message.Entry, 0),
		WorkerIDs:                make([]string, 0),
		WorkerStatus:             make(map[string]events.ProcessStatus),
		WorkerPhases:             make(map[string]events.ProcessPhase),
		WorkerMessages:           make(map[string][]chatrender.Message),
		WorkerMetrics:            make(map[string]*metrics.TokenMetrics),
		WorkerQueueCounts:        make(map[string]int),
		WorkerScrollPercents:     make(map[string]float64),
		CoordinatorScrollPercent: 0,
		MessageScrollPercent:     0,
		LastUpdated:              time.Time{},
	}
}

// IsEmpty returns true if the state has no content.
func (s *WorkflowUIState) IsEmpty() bool {
	return len(s.CoordinatorMessages) == 0 &&
		len(s.MessageEntries) == 0 &&
		len(s.WorkerIDs) == 0
}
