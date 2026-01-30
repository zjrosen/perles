package dashboard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMakeWorkflowZoneID(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{name: "index 0", index: 0, expected: "workflow:0"},
		{name: "index 5", index: 5, expected: "workflow:5"},
		{name: "large index", index: 100, expected: "workflow:100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeWorkflowZoneID(tt.index)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseWorkflowZoneID(t *testing.T) {
	tests := []struct {
		name          string
		zoneID        string
		expectedIndex int
		expectedOK    bool
	}{
		{name: "valid zone 0", zoneID: "workflow:0", expectedIndex: 0, expectedOK: true},
		{name: "valid zone 5", zoneID: "workflow:5", expectedIndex: 5, expectedOK: true},
		{name: "valid large zone", zoneID: "workflow:100", expectedIndex: 100, expectedOK: true},
		{name: "invalid prefix", zoneID: "tab:0", expectedIndex: 0, expectedOK: false},
		{name: "invalid format", zoneID: "workflow-0", expectedIndex: 0, expectedOK: false},
		{name: "non-numeric", zoneID: "workflow:abc", expectedIndex: 0, expectedOK: false},
		{name: "empty string", zoneID: "", expectedIndex: 0, expectedOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := parseWorkflowZoneID(tt.zoneID)
			require.Equal(t, tt.expectedOK, ok)
			if tt.expectedOK {
				require.Equal(t, tt.expectedIndex, index)
			}
		})
	}
}

func TestMakeTabZoneID(t *testing.T) {
	tests := []struct {
		name     string
		index    int
		expected string
	}{
		{name: "coordinator tab", index: TabCoordinator, expected: "tab:0"},
		{name: "observer tab", index: TabObserver, expected: "tab:1"},
		{name: "messages tab", index: TabMessages, expected: "tab:2"},
		{name: "first worker tab", index: TabFirstWorker, expected: "tab:3"},
		{name: "second worker tab", index: TabFirstWorker + 1, expected: "tab:4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := makeTabZoneID(tt.index)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTabZoneID(t *testing.T) {
	tests := []struct {
		name          string
		zoneID        string
		expectedIndex int
		expectedOK    bool
	}{
		{name: "valid tab 0", zoneID: "tab:0", expectedIndex: 0, expectedOK: true},
		{name: "valid tab 2", zoneID: "tab:2", expectedIndex: 2, expectedOK: true},
		{name: "invalid prefix", zoneID: "workflow:0", expectedIndex: 0, expectedOK: false},
		{name: "non-numeric", zoneID: "tab:abc", expectedIndex: 0, expectedOK: false},
		{name: "empty string", zoneID: "", expectedIndex: 0, expectedOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := parseTabZoneID(tt.zoneID)
			require.Equal(t, tt.expectedOK, ok)
			if tt.expectedOK {
				require.Equal(t, tt.expectedIndex, index)
			}
		})
	}
}

func TestZoneIDRoundtrip(t *testing.T) {
	for i := 0; i < 10; i++ {
		workflowZone := makeWorkflowZoneID(i)
		parsedIndex, ok := parseWorkflowZoneID(workflowZone)
		require.True(t, ok, "parseWorkflowZoneID should succeed for makeWorkflowZoneID output")
		require.Equal(t, i, parsedIndex)

		tabZone := makeTabZoneID(i)
		parsedTab, ok := parseTabZoneID(tabZone)
		require.True(t, ok, "parseTabZoneID should succeed for makeTabZoneID output")
		require.Equal(t, i, parsedTab)
	}
}
