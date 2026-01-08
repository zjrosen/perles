package mcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// ReviewAssignmentPromptSimple Tests
// ============================================================================

// TestReviewAssignmentPromptSimple_ReturnsNonEmpty verifies the function returns non-empty string.
func TestReviewAssignmentPromptSimple_ReturnsNonEmpty(t *testing.T) {
	prompt := ReviewAssignmentPromptSimple("perles-abc.1", "worker-1")
	require.NotEmpty(t, prompt, "Prompt should not be empty")
}

// TestReviewAssignmentPromptSimple_ContainsTaskIDAndImplementerID verifies parameters are used.
func TestReviewAssignmentPromptSimple_ContainsTaskIDAndImplementerID(t *testing.T) {
	taskID := "perles-xyz.42"
	implementerID := "worker-7"
	prompt := ReviewAssignmentPromptSimple(taskID, implementerID)

	require.Contains(t, prompt, taskID, "Prompt should contain taskID")
	require.Contains(t, prompt, implementerID, "Prompt should contain implementerID")
}

// TestReviewAssignmentPromptSimple_ContainsCriticalTestExecution verifies mandatory test language.
func TestReviewAssignmentPromptSimple_ContainsCriticalTestExecution(t *testing.T) {
	prompt := ReviewAssignmentPromptSimple("task-1", "worker-1")

	require.Contains(t, prompt, "CRITICAL: Run the tests",
		"Prompt MUST contain mandatory test execution language")
	require.Contains(t, prompt, "do not just read them",
		"Prompt MUST emphasize not just reading tests")
	require.Contains(t, prompt, "mandatory, not optional",
		"Prompt MUST clarify this is mandatory")
}

// TestReviewAssignmentPromptSimple_ContainsReportReviewVerdict verifies verdict call format.
func TestReviewAssignmentPromptSimple_ContainsReportReviewVerdict(t *testing.T) {
	prompt := ReviewAssignmentPromptSimple("task-1", "worker-1")

	require.Contains(t, prompt, "report_review_verdict",
		"Prompt should include report_review_verdict call")
}

// TestReviewAssignmentPromptSimple_CoversAllFourDimensions verifies all review dimensions.
func TestReviewAssignmentPromptSimple_CoversAllFourDimensions(t *testing.T) {
	prompt := ReviewAssignmentPromptSimple("task-1", "worker-1")

	dimensions := []string{
		"Correctness & Logic",
		"Tests",
		"Dead Code",
		"Acceptance Criteria",
	}

	for _, dimension := range dimensions {
		require.Contains(t, prompt, dimension,
			"Prompt should cover dimension: %s", dimension)
	}
}

// TestReviewAssignmentPromptSimple_IsShorterThanComplex verifies the simple prompt is shorter.
func TestReviewAssignmentPromptSimple_IsShorterThanComplex(t *testing.T) {
	simplePrompt := ReviewAssignmentPromptSimple("task-1", "worker-1")
	complexPrompt := ReviewAssignmentPrompt("task-1", "worker-1")

	simpleLines := len(strings.Split(simplePrompt, "\n"))
	complexLines := len(strings.Split(complexPrompt, "\n"))

	// Simple prompt should be significantly shorter (~70 lines vs ~180 lines)
	require.Less(t, simpleLines, complexLines,
		"Simple prompt (%d lines) should be shorter than complex prompt (%d lines)",
		simpleLines, complexLines)

	// Simple prompt should be between 60-80 lines
	require.GreaterOrEqual(t, simpleLines, 50,
		"Simple prompt should have at least 50 lines, got %d", simpleLines)
	require.LessOrEqual(t, simpleLines, 90,
		"Simple prompt should have at most 90 lines, got %d", simpleLines)
}

// ============================================================================
// CommitApprovalPrompt Tests (Updated for post_accountability_summary)
// ============================================================================

// TestCommitApprovalPrompt_UsesAccountabilitySummary verifies post_accountability_summary is referenced.
func TestCommitApprovalPrompt_UsesAccountabilitySummary(t *testing.T) {
	taskID := "perles-abc.1"
	prompt := CommitApprovalPrompt(taskID, "")

	require.Contains(t, prompt, "post_accountability_summary",
		"Prompt should include post_accountability_summary instruction")
	require.NotContains(t, prompt, "post_reflections",
		"Prompt should NOT include post_reflections (deprecated)")
}

// TestCommitApprovalPrompt_IncludesAccountabilityFields verifies all new fields are documented.
func TestCommitApprovalPrompt_IncludesAccountabilityFields(t *testing.T) {
	prompt := CommitApprovalPrompt("test-task", "")

	fields := []string{
		"task_id",
		"summary",
		"commits",
		"issues_closed",
		"issues_discovered",
		"verification_points",
		"retro",
		"next_steps",
	}

	for _, field := range fields {
		require.Contains(t, prompt, field,
			"Prompt should document field: %s", field)
	}
}

// TestCommitApprovalPrompt_IncludesRetroStructure verifies retro feedback structure.
func TestCommitApprovalPrompt_IncludesRetroStructure(t *testing.T) {
	prompt := CommitApprovalPrompt("test-task", "")

	// The example should show the retro structure
	require.Contains(t, prompt, "went_well", "Prompt should show went_well in retro")
	require.Contains(t, prompt, "friction", "Prompt should show friction in retro")
	require.Contains(t, prompt, "patterns", "Prompt should show patterns in retro")
}

// TestWorkerSystemPrompt_ContainsTraceContextDocs verifies trace context documentation.
func TestWorkerSystemPrompt_ContainsTraceContextDocs(t *testing.T) {
	prompt := WorkerSystemPrompt("worker-1")

	require.Contains(t, prompt, "Trace Context",
		"Prompt should document trace context")
	require.Contains(t, prompt, "trace_id",
		"Prompt should mention trace_id field")
	require.Contains(t, prompt, "backwards compatibility",
		"Prompt should mention backwards compatibility")
}
