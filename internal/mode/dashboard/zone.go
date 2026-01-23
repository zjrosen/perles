package dashboard

import (
	"fmt"
	"strconv"
	"strings"
)

// Zone ID format for dashboard mode:
// - Workflow rows: workflow:{index}
// - Coordinator tabs: tab:{index}
// - Chat input: chat-input

// Zone ID prefixes
const (
	zoneWorkflowPrefix = "workflow:"
	zoneTabPrefix      = "tab:"
	zoneChatInput      = "chat-input"
)

// makeWorkflowZoneID creates a zone ID for a workflow row.
func makeWorkflowZoneID(index int) string {
	return fmt.Sprintf("%s%d", zoneWorkflowPrefix, index)
}

// parseWorkflowZoneID extracts the index from a workflow zone ID.
// Returns (index, true) on success, or (0, false) on failure.
//
//nolint:unused // Used in zone_test.go for round-trip verification
func parseWorkflowZoneID(zoneID string) (int, bool) {
	if !strings.HasPrefix(zoneID, zoneWorkflowPrefix) {
		return 0, false
	}
	indexStr := strings.TrimPrefix(zoneID, zoneWorkflowPrefix)
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return 0, false
	}
	return index, true
}

// makeTabZoneID creates a zone ID for a coordinator panel tab.
func makeTabZoneID(index int) string {
	return fmt.Sprintf("%s%d", zoneTabPrefix, index)
}

// parseTabZoneID extracts the index from a tab zone ID.
// Returns (index, true) on success, or (0, false) on failure.
//
//nolint:unused // Used in zone_test.go for round-trip verification
func parseTabZoneID(zoneID string) (int, bool) {
	if !strings.HasPrefix(zoneID, zoneTabPrefix) {
		return 0, false
	}
	indexStr := strings.TrimPrefix(zoneID, zoneTabPrefix)
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return 0, false
	}
	return index, true
}
