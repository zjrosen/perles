package beads

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUpdateStatus_InvalidIssue tests error handling for invalid issue IDs.
func TestUpdateStatus_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := UpdateStatus("nonexistent-xyz", StatusInProgress)
	assert.Error(t, err, "expected error for nonexistent issue")
}

// TestUpdatePriority_InvalidIssue tests error handling for invalid issue IDs.
func TestUpdatePriority_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := UpdatePriority("nonexistent-xyz", PriorityHigh)
	assert.Error(t, err, "expected error for nonexistent issue")
}

// TestCloseIssue_InvalidIssue tests error handling for invalid issue IDs.
func TestCloseIssue_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := CloseIssue("nonexistent-xyz", "testing")
	assert.Error(t, err, "expected error for nonexistent issue")
}

// TestReopenIssue_InvalidIssue tests error handling for invalid issue IDs.
func TestReopenIssue_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := ReopenIssue("nonexistent-xyz")
	assert.Error(t, err, "expected error for nonexistent issue")
}

// TestDeleteIssue_InvalidIssue tests error handling for invalid issue IDs.
func TestDeleteIssue_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := DeleteIssue("nonexistent-xyz")
	assert.Error(t, err, "expected error for nonexistent issue")
}

// TestDeleteIssueCascade_InvalidIssue tests error handling for invalid issue IDs.
func TestDeleteIssueCascade_InvalidIssue(t *testing.T) {
	// Check if bd command is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping integration test")
	}

	err := DeleteIssueCascade("nonexistent-xyz")
	assert.Error(t, err, "expected error for nonexistent issue")
}
