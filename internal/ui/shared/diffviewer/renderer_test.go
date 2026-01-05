package diffviewer

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/mocks"
)

func TestRenderDiffContent_Colors(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				OldStart: 10,
				OldCount: 5,
				NewStart: 10,
				NewCount: 6,
				Header:   "@@ -10,5 +10,6 @@ func example()",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func example()"},
					{Type: LineContext, OldLineNum: 10, NewLineNum: 10, Content: "context line"},
					{Type: LineDeletion, OldLineNum: 11, NewLineNum: 0, Content: "deleted line"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 11, Content: "added line"},
					{Type: LineContext, OldLineNum: 12, NewLineNum: 12, Content: "more context"},
				},
			},
		},
		Additions: 1,
		Deletions: 1,
	}

	result := renderDiffContentWithWordDiff(file, nil, 80, 20)
	require.NotEmpty(t, result)

	// Content should contain the line content
	require.Contains(t, result, "context line")
	require.Contains(t, result, "deleted line")
	require.Contains(t, result, "added line")

	// Should contain the prefixes
	require.Contains(t, result, "+")
	require.Contains(t, result, "-")
}

func TestRenderDiffContent_LineNumbers(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				OldStart: 100,
				OldCount: 3,
				NewStart: 100,
				NewCount: 3,
				Header:   "@@ -100,3 +100,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineContext, OldLineNum: 100, NewLineNum: 100, Content: "line 100"},
					{Type: LineContext, OldLineNum: 101, NewLineNum: 101, Content: "line 101"},
					{Type: LineContext, OldLineNum: 102, NewLineNum: 102, Content: "line 102"},
				},
			},
		},
	}

	result := renderDiffContentWithWordDiff(file, nil, 80, 20)

	// Should show line numbers
	require.Contains(t, result, "100")
	require.Contains(t, result, "101")
	require.Contains(t, result, "102")
}

func TestRenderDiffContent_BinaryFile(t *testing.T) {
	file := DiffFile{
		OldPath:  "image.png",
		NewPath:  "image.png",
		IsBinary: true,
	}

	result := renderDiffContentWithWordDiff(file, nil, 60, 10)
	require.Contains(t, result, "Binary file")
	require.Contains(t, result, "cannot display diff")
}

func TestRenderDiffContent_NoHunks(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks:   []DiffHunk{},
	}

	result := renderDiffContentWithWordDiff(file, nil, 60, 10)
	require.Contains(t, result, "No changes")
}

func TestRenderDiffContent_ZeroDimensions(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{Lines: []DiffLine{{Type: LineContext, Content: "test"}}},
		},
	}

	// Zero width
	result := renderDiffContentWithWordDiff(file, nil, 0, 10)
	require.Empty(t, result)

	// Zero height is allowed (means viewport handles scrolling)
	result = renderDiffContentWithWordDiff(file, nil, 10, 0)
	require.NotEmpty(t, result)
}

func TestRenderDiffContent_LongLine(t *testing.T) {
	longContent := strings.Repeat("x", 200)
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: longContent},
				},
			},
		},
	}

	result := renderDiffContentWithWordDiff(file, nil, 80, 10)
	// Should not have lines longer than width (truncation applied)
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		width := lipgloss.Width(line)
		require.LessOrEqual(t, width, 80, "line should not exceed width")
	}
}

// Helper to create a sample diff file for golden tests
func createSampleDiffFile() DiffFile {
	return DiffFile{
		OldPath:   "internal/example.go",
		NewPath:   "internal/example.go",
		Additions: 3,
		Deletions: 2,
		Hunks: []DiffHunk{
			{
				OldStart: 10,
				OldCount: 6,
				NewStart: 10,
				NewCount: 7,
				Header:   "@@ -10,6 +10,7 @@ func example() {",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func example() {"},
					{Type: LineContext, OldLineNum: 10, NewLineNum: 10, Content: "\treturn nil"},
					{Type: LineDeletion, OldLineNum: 11, NewLineNum: 0, Content: "\t// old comment"},
					{Type: LineDeletion, OldLineNum: 12, NewLineNum: 0, Content: "\toldCode()"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 11, Content: "\t// new comment"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 12, Content: "\tnewCode()"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 13, Content: "\textraLine()"},
					{Type: LineContext, OldLineNum: 13, NewLineNum: 14, Content: "}"},
				},
			},
		},
	}
}

// Golden tests for visual output verification

func TestView_Golden_DiffContent(t *testing.T) {
	file := createSampleDiffFile()
	result := renderDiffContentWithWordDiff(file, nil, 80, 20)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_Binary(t *testing.T) {
	file := DiffFile{
		OldPath:  "image.png",
		NewPath:  "image.png",
		IsBinary: true,
	}
	result := renderDiffContentWithWordDiff(file, nil, 60, 10)
	teatest.RequireEqualOutput(t, []byte(result))
}

// Test internal helper functions

func TestFormatStats(t *testing.T) {
	tests := []struct {
		name       string
		additions  int
		deletions  int
		binary     bool
		contains   []string
		notContain []string
	}{
		{
			"both adds and dels",
			10, 5, false,
			[]string{"+10", "-5"},
			nil,
		},
		{
			"only additions",
			10, 0, false,
			[]string{"+10"},
			[]string{"-0"},
		},
		{
			"only deletions",
			0, 5, false,
			[]string{"-5"},
			[]string{"+0"},
		},
		{
			"binary",
			0, 0, true,
			[]string{"binary"},
			[]string{"+", "-"},
		},
		{
			"no changes",
			0, 0, false,
			nil,
			[]string{"+", "-"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatStats(tc.additions, tc.deletions, tc.binary)
			for _, s := range tc.contains {
				require.Contains(t, result, s)
			}
			for _, s := range tc.notContain {
				require.NotContains(t, result, s)
			}
		})
	}
}

func TestFormatGutter(t *testing.T) {
	tests := []struct {
		name       string
		oldLineNum int
		newLineNum int
		contains   string
	}{
		{"context line", 10, 10, "10"},
		{"addition only new", 0, 15, "15"},
		{"deletion only old", 20, 0, "20"},
		{"both zero", 0, 0, "|"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatGutter(tc.oldLineNum, tc.newLineNum)
			require.Contains(t, result, tc.contains)
			require.Contains(t, result, "|") // All gutters have separator
		})
	}
}

// Tests for enhanced empty, error, and loading states

func TestRenderEnhancedEmptyState(t *testing.T) {
	result := renderEnhancedEmptyState(80, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "No changes to display")
	require.Contains(t, result, "working directory is clean")
	require.Contains(t, result, "Tips:")
	require.Contains(t, result, "Commits, Branches, and Worktrees tabs")
	require.Contains(t, result, "[?]")
}

func TestRenderEnhancedEmptyState_ZeroDimensions(t *testing.T) {
	// Zero width
	result := renderEnhancedEmptyState(0, 20)
	require.Empty(t, result)

	// Zero height
	result = renderEnhancedEmptyState(80, 0)
	require.Empty(t, result)
}

func TestRenderErrorState(t *testing.T) {
	tests := []struct {
		name         string
		category     ErrorCategory
		message      string
		helpText     string
		wantContains []string
	}{
		{
			name:     "parse error",
			category: ErrCategoryParse,
			message:  "Failed to parse diff output",
			helpText: "The diff output format was unexpected.",
			wantContains: []string{
				"Parse Error",
				"Failed to parse diff output",
				"[v]", // View raw output
				"[?]", // Get help
			},
		},
		{
			name:     "git operation error",
			category: ErrCategoryGitOp,
			message:  "Git command failed",
			wantContains: []string{
				"Git Operation Error",
				"Git command failed",
				"[r]", // Retry
				"[?]", // Get help
			},
		},
		{
			name:     "permission error",
			category: ErrCategoryPermission,
			message:  "Permission denied",
			wantContains: []string{
				"Permission Error",
				"Permission denied",
				"[?]", // Get help (only action for permission errors)
			},
		},
		{
			name:     "conflict error",
			category: ErrCategoryConflict,
			message:  "Merge conflict detected",
			wantContains: []string{
				"Conflict Error",
				"Merge conflict detected",
				"[r]", // Reload
				"[?]", // Get help
			},
		},
		{
			name:     "timeout error",
			category: ErrCategoryTimeout,
			message:  "Operation timed out after 5 seconds",
			helpText: "This may happen with very large repositories.",
			wantContains: []string{
				"Timeout Error",
				"Operation timed out",
				"large repositories",
				"[r]", // Retry with longer timeout
				"[?]", // Get help
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			diffErr := NewDiffError(tc.category, tc.message)
			if tc.helpText != "" {
				diffErr = diffErr.WithHelpText(tc.helpText)
			}

			result := renderErrorState(diffErr, 80, 20)
			require.NotEmpty(t, result)

			for _, want := range tc.wantContains {
				require.Contains(t, result, want, "expected result to contain %q", want)
			}
		})
	}
}

func TestRenderErrorState_ZeroDimensions(t *testing.T) {
	diffErr := NewDiffError(ErrCategoryGitOp, "Test error")

	// Zero width
	result := renderErrorState(diffErr, 0, 20)
	require.Empty(t, result)

	// Zero height
	result = renderErrorState(diffErr, 80, 0)
	require.Empty(t, result)
}

func TestRenderEnhancedLoadingState(t *testing.T) {
	result := renderEnhancedLoadingState(80, 20, "Loading diff...")
	require.NotEmpty(t, result)
	require.Contains(t, result, "Loading diff...")
	require.Contains(t, result, "may take a moment")
}

func TestRenderEnhancedLoadingState_DefaultMessage(t *testing.T) {
	result := renderEnhancedLoadingState(80, 20, "")
	require.NotEmpty(t, result)
	require.Contains(t, result, "Loading diff...")
}

func TestRenderEnhancedLoadingState_ZeroDimensions(t *testing.T) {
	// Zero width
	result := renderEnhancedLoadingState(0, 20, "Loading...")
	require.Empty(t, result)

	// Zero height
	result = renderEnhancedLoadingState(80, 0, "Loading...")
	require.Empty(t, result)
}

func TestGetRecoveryActions(t *testing.T) {
	tests := []struct {
		name         string
		category     ErrorCategory
		wantActions  []string
		wantMinCount int
	}{
		{
			name:         "parse error has view raw",
			category:     ErrCategoryParse,
			wantActions:  []string{"[v]", "[?]"},
			wantMinCount: 2,
		},
		{
			name:         "git op error has retry",
			category:     ErrCategoryGitOp,
			wantActions:  []string{"[r]", "[?]"},
			wantMinCount: 2,
		},
		{
			name:         "permission error has only help",
			category:     ErrCategoryPermission,
			wantActions:  []string{"[?]"},
			wantMinCount: 1,
		},
		{
			name:         "conflict error has reload",
			category:     ErrCategoryConflict,
			wantActions:  []string{"[r]", "[?]"},
			wantMinCount: 2,
		},
		{
			name:         "timeout error has retry",
			category:     ErrCategoryTimeout,
			wantActions:  []string{"[r]", "[?]"},
			wantMinCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actions := getRecoveryActions(tc.category)
			require.GreaterOrEqual(t, len(actions), tc.wantMinCount)

			// Check that expected actions are present
			actionKeys := make([]string, len(actions))
			for i, a := range actions {
				actionKeys[i] = a.Key
			}
			for _, want := range tc.wantActions {
				require.Contains(t, actionKeys, want, "expected actions to contain %q", want)
			}
		})
	}
}

// Golden tests for enhanced states

func TestView_Golden_EnhancedEmpty(t *testing.T) {
	result := renderEnhancedEmptyState(60, 15)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_ErrorState_GitOp(t *testing.T) {
	diffErr := NewDiffError(ErrCategoryGitOp, "Git operation failed").
		WithHelpText("Check your git configuration and try again.")
	result := renderErrorState(diffErr, 60, 15)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_ErrorState_Timeout(t *testing.T) {
	diffErr := NewDiffError(ErrCategoryTimeout, "Operation timed out after 5 seconds").
		WithHelpText("This may happen with very large repositories.")
	result := renderErrorState(diffErr, 60, 15)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_EnhancedLoading(t *testing.T) {
	result := renderEnhancedLoadingState(60, 15, "Loading commit diff...")
	teatest.RequireEqualOutput(t, []byte(result))
}

// Tests for word-level diff rendering

func TestRenderDiffContentWithWordDiff_Basic(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,2 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineDeletion, OldLineNum: 1, NewLineNum: 0, Content: "func oldName() {"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 1, Content: "func newName() {"},
				},
			},
		},
	}

	// Compute word diff
	wordDiff := computeFileWordDiff(file)

	result := renderDiffContentWithWordDiff(file, &wordDiff, 80, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "oldName")
	require.Contains(t, result, "newName")
	require.Contains(t, result, "-")
	require.Contains(t, result, "+")
}

func TestRenderDiffContentWithWordDiff_NilWordDiff(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,2 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineDeletion, OldLineNum: 1, NewLineNum: 0, Content: "deleted line"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 1, Content: "added line"},
				},
			},
		},
	}

	// Pass nil word diff - should fall back to standard rendering
	result := renderDiffContentWithWordDiff(file, nil, 80, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "deleted line")
	require.Contains(t, result, "added line")
}

func TestRenderDiffContentWithWordDiff_Binary(t *testing.T) {
	file := DiffFile{
		OldPath:  "image.png",
		NewPath:  "image.png",
		IsBinary: true,
	}

	result := renderDiffContentWithWordDiff(file, nil, 60, 10)
	require.Contains(t, result, "Binary file")
}

func TestRenderDiffContentWithWordDiff_NoHunks(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks:   []DiffHunk{},
	}

	result := renderDiffContentWithWordDiff(file, nil, 60, 10)
	require.Contains(t, result, "No changes")
}

func TestRenderDiffContentWithWordDiff_ZeroWidth(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks:   []DiffHunk{{Lines: []DiffLine{{Type: LineContext, Content: "test"}}}},
	}

	result := renderDiffContentWithWordDiff(file, nil, 0, 10)
	require.Empty(t, result)
}

func TestRenderSegments(t *testing.T) {
	unchangedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#000000"))
	changedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Background(lipgloss.Color("#440000"))

	tests := []struct {
		name     string
		segments []wordSegment
		contains []string
	}{
		{
			name: "all unchanged",
			segments: []wordSegment{
				{Type: segmentUnchanged, Text: "hello"},
				{Type: segmentUnchanged, Text: " "},
				{Type: segmentUnchanged, Text: "world"},
			},
			contains: []string{"hello", " ", "world"},
		},
		{
			name: "mixed unchanged and added",
			segments: []wordSegment{
				{Type: segmentUnchanged, Text: "func "},
				{Type: segmentAdded, Text: "newName"},
				{Type: segmentUnchanged, Text: "()"},
			},
			contains: []string{"func ", "newName", "()"},
		},
		{
			name: "mixed unchanged and deleted",
			segments: []wordSegment{
				{Type: segmentUnchanged, Text: "func "},
				{Type: segmentDeleted, Text: "oldName"},
				{Type: segmentUnchanged, Text: "()"},
			},
			contains: []string{"func ", "oldName", "()"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := renderSegments(tc.segments, unchangedStyle, changedStyle)
			for _, want := range tc.contains {
				require.Contains(t, result, want)
			}
		})
	}
}

func TestRenderSegments_EmptySegments(t *testing.T) {
	unchangedStyle := lipgloss.NewStyle()
	changedStyle := lipgloss.NewStyle()

	result := renderSegments([]wordSegment{}, unchangedStyle, changedStyle)
	require.Empty(t, result)
}

// Golden test for word diff rendering
func TestView_Golden_DiffContentWithWordDiff(t *testing.T) {
	file := DiffFile{
		OldPath: "internal/example.go",
		NewPath: "internal/example.go",
		Hunks: []DiffHunk{
			{
				OldStart: 10,
				OldCount: 3,
				NewStart: 10,
				NewCount: 3,
				Header:   "@@ -10,3 +10,3 @@ func example() {",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func example() {"},
					{Type: LineDeletion, OldLineNum: 10, NewLineNum: 0, Content: "\treturn oldValue"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 10, Content: "\treturn newValue"},
					{Type: LineContext, OldLineNum: 11, NewLineNum: 11, Content: "}"},
				},
			},
		},
	}

	wordDiff := computeFileWordDiff(file)
	result := renderDiffContentWithWordDiff(file, &wordDiff, 80, 20)
	teatest.RequireEqualOutput(t, []byte(result))
}

// Tests for side-by-side rendering

func TestRenderDiffContentSideBySide_Basic(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				OldStart: 1,
				OldCount: 3,
				NewStart: 1,
				NewCount: 3,
				Header:   "@@ -1,3 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "unchanged line"},
					{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "old content"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "new content"},
					{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "more unchanged"},
				},
			},
		},
	}

	result := renderDiffContentSideBySide(file, nil, 120, 20)
	require.NotEmpty(t, result)

	// Should contain separator (side-by-side layout)
	require.Contains(t, result, "│")

	// Should contain content
	require.Contains(t, result, "unchanged line")
	require.Contains(t, result, "old content")
	require.Contains(t, result, "new content")
}

func TestRenderDiffContentSideBySide_ZeroWidth(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks:   []DiffHunk{{Lines: []DiffLine{{Type: LineContext, Content: "test"}}}},
	}

	result := renderDiffContentSideBySide(file, nil, 0, 10)
	require.Empty(t, result)
}

func TestRenderDiffContentSideBySide_NarrowWidth(t *testing.T) {
	// Width too narrow for side-by-side should fall back to unified
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "test"},
				},
			},
		},
	}

	// Width of 50 is too narrow (min is ~91 with our constants)
	result := renderDiffContentSideBySide(file, nil, 50, 10)
	require.NotEmpty(t, result)
	// Should not contain side-by-side separator (fell back to unified)
	// The result should just show regular unified diff
	require.Contains(t, result, "test")
}

func TestRenderDiffContentSideBySide_BinaryFile(t *testing.T) {
	file := DiffFile{
		OldPath:  "image.png",
		NewPath:  "image.png",
		IsBinary: true,
	}

	result := renderDiffContentSideBySide(file, nil, 120, 10)
	require.Contains(t, result, "Binary file")
}

func TestRenderDiffContentSideBySide_NoHunks(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks:   []DiffHunk{},
	}

	result := renderDiffContentSideBySide(file, nil, 120, 10)
	require.Contains(t, result, "No changes")
}

func TestRenderDiffContentSideBySide_PureAdditions(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -0,0 +1,2 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineAddition, NewLineNum: 1, Content: "new line 1"},
					{Type: LineAddition, NewLineNum: 2, Content: "new line 2"},
				},
			},
		},
	}

	result := renderDiffContentSideBySide(file, nil, 120, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "new line 1")
	require.Contains(t, result, "new line 2")
}

func TestRenderDiffContentSideBySide_PureDeletions(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +0,0 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineDeletion, OldLineNum: 1, Content: "old line 1"},
					{Type: LineDeletion, OldLineNum: 2, Content: "old line 2"},
				},
			},
		},
	}

	result := renderDiffContentSideBySide(file, nil, 120, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "old line 1")
	require.Contains(t, result, "old line 2")
}

func TestRenderDiffContentSideBySide_WithWordDiff(t *testing.T) {
	file := DiffFile{
		OldPath: "file.go",
		NewPath: "file.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: ""},
					{Type: LineDeletion, OldLineNum: 1, Content: "func oldName() {"},
					{Type: LineAddition, NewLineNum: 1, Content: "func newName() {"},
				},
			},
		},
	}

	wordDiff := computeFileWordDiff(file)
	result := renderDiffContentSideBySide(file, &wordDiff, 120, 20)
	require.NotEmpty(t, result)
	require.Contains(t, result, "oldName")
	require.Contains(t, result, "newName")
}

func TestRenderSideBySideLine(t *testing.T) {
	lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	line := &DiffLine{
		Type:       LineDeletion,
		OldLineNum: 42,
		Content:    "test content",
	}

	result := renderSideBySideLine(line, 30, lineStyle, gutterStyle, nil, lineStyle)

	// Should contain line number
	require.Contains(t, result, "42")

	// Should contain content
	require.Contains(t, result, "test content")

	// Width should be exactly sideBySideGutterWidth + content
	require.Equal(t, sideBySideGutterWidth+30, lipgloss.Width(result))
}

func TestRenderSideBySideLine_NilLine(t *testing.T) {
	lineStyle := lipgloss.NewStyle()
	gutterStyle := lipgloss.NewStyle()

	result := renderSideBySideLine(nil, 30, lineStyle, gutterStyle, nil, lineStyle)

	// Should be all spaces
	require.Equal(t, strings.Repeat(" ", sideBySideGutterWidth+30), result)
}

func TestRenderSideBySideEmptyLine(t *testing.T) {
	emptyStyle := lipgloss.NewStyle()
	gutterStyle := lipgloss.NewStyle()

	result := renderSideBySideEmptyLine(5, 30, emptyStyle, gutterStyle)

	// Width should be exactly gutter + content
	require.Equal(t, 35, lipgloss.Width(result))
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected int
	}{
		{"short string", "hi", 10, 10},
		{"exact width", "hello", 5, 5},
		{"longer than width", "hello world", 5, 11}, // No truncation, just returns as-is
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := padRight(tc.input, tc.width)
			require.Equal(t, tc.expected, lipgloss.Width(result))
		})
	}
}

func TestFindLineIndex(t *testing.T) {
	lines := []DiffLine{
		{Type: LineContext, Content: "line 0"},
		{Type: LineDeletion, Content: "line 1"},
		{Type: LineAddition, Content: "line 2"},
	}

	// Find existing lines
	require.Equal(t, 0, findLineIndex(lines, &lines[0]))
	require.Equal(t, 1, findLineIndex(lines, &lines[1]))
	require.Equal(t, 2, findLineIndex(lines, &lines[2]))

	// Find nil returns -1
	require.Equal(t, -1, findLineIndex(lines, nil))

	// Find non-existent line returns -1
	other := DiffLine{Type: LineContext, Content: "other"}
	require.Equal(t, -1, findLineIndex(lines, &other))
}

// Golden test for side-by-side view at 120 columns
func TestView_Golden_SideBySide_120col(t *testing.T) {
	file := createSampleDiffFile()
	result := renderDiffContentSideBySide(file, nil, 120, 20)
	teatest.RequireEqualOutput(t, []byte(result))
}

// Golden test for side-by-side view with word diff
func TestView_Golden_SideBySide_WithWordDiff(t *testing.T) {
	file := DiffFile{
		OldPath: "internal/example.go",
		NewPath: "internal/example.go",
		Hunks: []DiffHunk{
			{
				OldStart: 10,
				OldCount: 3,
				NewStart: 10,
				NewCount: 3,
				Header:   "@@ -10,3 +10,3 @@ func example() {",
				Lines: []DiffLine{
					{Type: LineHunkHeader, Content: "func example() {"},
					{Type: LineDeletion, OldLineNum: 10, NewLineNum: 0, Content: "\treturn oldValue"},
					{Type: LineAddition, OldLineNum: 0, NewLineNum: 10, Content: "\treturn newValue"},
					{Type: LineContext, OldLineNum: 11, NewLineNum: 11, Content: "}"},
				},
			},
		},
	}

	wordDiff := computeFileWordDiff(file)
	result := renderDiffContentSideBySide(file, &wordDiff, 120, 20)
	teatest.RequireEqualOutput(t, []byte(result))
}

// =============================================================================
// renderFileTree Golden Tests
// =============================================================================

// Test helper to create a file tree with various structures
func createEmptyFileTree() *FileTree {
	return NewFileTree([]DiffFile{})
}

func createSingleFileTree() *FileTree {
	return NewFileTree([]DiffFile{
		{
			NewPath:   "main.go",
			Additions: 10,
			Deletions: 5,
		},
	})
}

func createNestedDirectoryTree() *FileTree {
	return NewFileTree([]DiffFile{
		{
			NewPath:   "cmd/app/main.go",
			Additions: 25,
			Deletions: 10,
		},
		{
			NewPath:   "cmd/app/config.go",
			Additions: 15,
			Deletions: 5,
		},
		{
			NewPath:   "internal/pkg/utils.go",
			Additions: 50,
			Deletions: 20,
		},
		{
			NewPath:   "README.md",
			Additions: 5,
			Deletions: 0,
		},
	})
}

func createMixedStatusTree() *FileTree {
	return NewFileTree([]DiffFile{
		{
			NewPath:   "added_file.go",
			IsNew:     true,
			Additions: 100,
		},
		{
			OldPath:   "deleted_file.go",
			IsDeleted: true,
			Deletions: 50,
		},
		{
			NewPath:   "modified.go",
			Additions: 10,
			Deletions: 5,
		},
		{
			OldPath:   "old_name.go",
			NewPath:   "new_name.go",
			IsRenamed: true,
			Additions: 3,
			Deletions: 2,
		},
		{
			NewPath:  "image.png",
			IsBinary: true,
		},
	})
}

// TestRenderFileTree_EmptyTree tests rendering an empty file tree
func TestRenderFileTree_EmptyTree(t *testing.T) {
	tree := createEmptyFileTree()
	result := renderFileTree(tree, 0, 0, 40, 10, true)
	require.Contains(t, result, "No files changed")
}

// TestRenderFileTree_SingleFile tests rendering a single file (no directories)
func TestRenderFileTree_SingleFile(t *testing.T) {
	tree := createSingleFileTree()
	result := renderFileTree(tree, 0, 0, 40, 10, true)
	require.Contains(t, result, "main.go")
	require.Contains(t, result, "+10")
	require.Contains(t, result, "-5")
}

// TestRenderFileTree_NestedDirectories tests rendering nested directories
func TestRenderFileTree_NestedDirectories(t *testing.T) {
	tree := createNestedDirectoryTree()
	result := renderFileTree(tree, 0, 0, 50, 15, true)

	// Should show directory names
	require.Contains(t, result, "cmd")
	require.Contains(t, result, "internal")

	// Should show files
	require.Contains(t, result, "main.go")
	require.Contains(t, result, "config.go")
	require.Contains(t, result, "utils.go")
	require.Contains(t, result, "README.md")
}

// TestRenderFileTree_CollapsedDirectory tests rendering with a collapsed directory
func TestRenderFileTree_CollapsedDirectory(t *testing.T) {
	tree := createNestedDirectoryTree()

	// Collapse the cmd directory
	nodes := tree.VisibleNodes()
	for _, node := range nodes {
		if node.Name == "cmd" && node.IsDir {
			tree.Toggle(node)
			break
		}
	}

	result := renderFileTree(tree, 0, 0, 50, 15, true)

	// Should show cmd directory with collapsed indicator
	require.Contains(t, result, "cmd")
	require.Contains(t, result, treeCollapsedIcon)

	// Should NOT show children of cmd when collapsed
	// After collapsing, main.go and config.go should not appear in visible nodes
	// but other files like README.md and internal/* should still appear
	require.Contains(t, result, "README.md")
}

// TestRenderFileTree_ExpandedDirectory tests rendering with expanded directories
func TestRenderFileTree_ExpandedDirectory(t *testing.T) {
	tree := createNestedDirectoryTree()
	result := renderFileTree(tree, 0, 0, 50, 15, true)

	// All directories start expanded, so we should see the expanded icon
	require.Contains(t, result, treeExpandedIcon)
}

// TestRenderFileTree_SelectionFocused tests selection highlighting when focused
func TestRenderFileTree_SelectionFocused(t *testing.T) {
	tree := createSingleFileTree()

	// Render with selection at index 0, focused=true
	result := renderFileTree(tree, 0, 0, 40, 10, true)
	require.NotEmpty(t, result)
	require.Contains(t, result, "main.go")
}

// TestRenderFileTree_SelectionUnfocused tests selection highlighting when unfocused
func TestRenderFileTree_SelectionUnfocused(t *testing.T) {
	tree := createSingleFileTree()

	// Render with selection at index 0, focused=false
	result := renderFileTree(tree, 0, 0, 40, 10, false)
	require.NotEmpty(t, result)
	require.Contains(t, result, "main.go")
}

// TestRenderFileTree_ZeroDimensions tests zero dimensions
func TestRenderFileTree_ZeroDimensions(t *testing.T) {
	tree := createSingleFileTree()

	// Zero width
	result := renderFileTree(tree, 0, 0, 0, 10, true)
	require.Empty(t, result)

	// Zero height
	result = renderFileTree(tree, 0, 0, 40, 0, true)
	require.Empty(t, result)
}

// TestRenderFileTree_Scrolling tests scrolling behavior
func TestRenderFileTree_Scrolling(t *testing.T) {
	tree := createNestedDirectoryTree()

	// Render with scrollTop=2, should skip first 2 nodes
	result := renderFileTree(tree, 3, 2, 50, 5, true)
	require.NotEmpty(t, result)
}

// TestRenderFileTree_MixedFileStatuses tests files with different statuses
func TestRenderFileTree_MixedFileStatuses(t *testing.T) {
	tree := createMixedStatusTree()
	result := renderFileTree(tree, 0, 0, 50, 15, true)

	// Should show status indicators
	require.Contains(t, result, "A") // Added
	require.Contains(t, result, "D") // Deleted
	require.Contains(t, result, "M") // Modified
	require.Contains(t, result, "R") // Renamed
	require.Contains(t, result, "B") // Binary
}

// Golden tests for renderFileTree visual output

func TestView_Golden_FileTree_Empty(t *testing.T) {
	tree := createEmptyFileTree()
	result := renderFileTree(tree, 0, 0, 40, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_SingleFile(t *testing.T) {
	tree := createSingleFileTree()
	result := renderFileTree(tree, 0, 0, 40, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_SingleFile_Unfocused(t *testing.T) {
	tree := createSingleFileTree()
	result := renderFileTree(tree, 0, 0, 40, 10, false)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_NestedDirs_Expanded(t *testing.T) {
	tree := createNestedDirectoryTree()
	result := renderFileTree(tree, 0, 0, 50, 15, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_NestedDirs_Collapsed(t *testing.T) {
	tree := createNestedDirectoryTree()

	// Collapse the cmd directory
	nodes := tree.VisibleNodes()
	for _, node := range nodes {
		if node.Name == "cmd" && node.IsDir {
			tree.Toggle(node)
			break
		}
	}

	result := renderFileTree(tree, 0, 0, 50, 15, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_MixedStatus(t *testing.T) {
	tree := createMixedStatusTree()
	result := renderFileTree(tree, 0, 0, 50, 15, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_Selection_Focused(t *testing.T) {
	tree := createNestedDirectoryTree()
	// Select the 3rd item (index 2)
	result := renderFileTree(tree, 2, 0, 50, 15, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_Selection_Unfocused(t *testing.T) {
	tree := createNestedDirectoryTree()
	// Select the 3rd item (index 2), but unfocused
	result := renderFileTree(tree, 2, 0, 50, 15, false)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FileTree_DeepNesting(t *testing.T) {
	// Create a deeply nested file structure (3 levels)
	tree := NewFileTree([]DiffFile{
		{
			NewPath:   "src/components/ui/button/button.go",
			Additions: 50,
			Deletions: 10,
		},
		{
			NewPath:   "src/components/ui/button/button_test.go",
			Additions: 100,
			Deletions: 0,
		},
		{
			NewPath:   "src/components/ui/modal/modal.go",
			Additions: 75,
			Deletions: 25,
		},
		{
			NewPath:   "src/utils/helpers.go",
			Additions: 20,
			Deletions: 5,
		},
	})
	result := renderFileTree(tree, 0, 0, 60, 20, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

// =============================================================================
// renderDirectoryDiffContent Tests
// =============================================================================

// Helper to create a directory node with files for testing
func createDirNodeWithFiles(files []DiffFile) *FileTreeNode {
	if len(files) == 0 {
		return &FileTreeNode{
			Name:     "testdir",
			Path:     "testdir",
			IsDir:    true,
			Expanded: true,
			Children: []*FileTreeNode{},
		}
	}

	// Create file nodes as children
	children := make([]*FileTreeNode, len(files))
	for i := range files {
		children[i] = &FileTreeNode{
			Name:  files[i].NewPath,
			Path:  "testdir/" + files[i].NewPath,
			IsDir: false,
			File:  &files[i],
			Depth: 1,
		}
	}

	return &FileTreeNode{
		Name:     "testdir",
		Path:     "testdir",
		IsDir:    true,
		Expanded: true,
		Children: children,
	}
}

// Helper to create sample diff files for testing
func createSampleDiffFiles() []DiffFile {
	return []DiffFile{
		{
			OldPath:   "file1.go",
			NewPath:   "file1.go",
			Additions: 10,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					Header:   "@@ -1,3 +1,4 @@ func main()",
					OldStart: 1,
					OldCount: 3,
					NewStart: 1,
					NewCount: 4,
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "func main()"},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "    fmt.Println(\"hello\")"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "    return"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "    fmt.Println(\"world\")"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "    return nil"},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 4, Content: "}"},
					},
				},
			},
		},
	}
}

// TestRenderDirectoryDiffContent tests the renderDirectoryDiffContent function
func TestRenderDirectoryDiffContent(t *testing.T) {
	tests := []struct {
		name     string
		files    []DiffFile
		width    int
		contains []string
		equals   string // If non-empty, check exact match
	}{
		{
			name:   "empty directory",
			files:  []DiffFile{},
			width:  80,
			equals: "No files in directory",
		},
		{
			name: "single file with hunks",
			files: []DiffFile{
				{
					NewPath:   "main.go",
					Additions: 5,
					Deletions: 2,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,2 +1,3 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
								{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "// old comment"},
								{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "// new comment"},
								{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "// extra line"},
							},
						},
					},
				},
			},
			width: 80,
			contains: []string{
				"main.go",        // File header
				"+5",             // Additions stat
				"-2",             // Deletions stat
				"package main",   // Context content
				"// old comment", // Deleted content
				"// new comment", // Added content
			},
		},
		{
			name: "multiple files",
			files: []DiffFile{
				{
					NewPath:   "file1.go",
					Additions: 10,
					Deletions: 3,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "first file"},
							},
						},
					},
				},
				{
					NewPath:   "file2.go",
					Additions: 5,
					Deletions: 1,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "second file"},
							},
						},
					},
				},
			},
			width: 80,
			contains: []string{
				"file1.go",    // First file header
				"file2.go",    // Second file header
				"first file",  // First file content
				"second file", // Second file content
			},
		},
		{
			name: "deleted file uses OldPath",
			files: []DiffFile{
				{
					OldPath:   "deleted_file.go",
					NewPath:   "/dev/null",
					IsDeleted: true,
					Deletions: 10,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,2 +0,0 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "deleted content"},
								{Type: LineDeletion, OldLineNum: 2, Content: "more deleted"},
							},
						},
					},
				},
			},
			width: 80,
			contains: []string{
				"deleted_file.go", // Should use OldPath for deleted files
				"deleted content",
			},
		},
		{
			name: "binary file",
			files: []DiffFile{
				{
					NewPath:  "image.png",
					IsBinary: true,
				},
			},
			width: 80,
			contains: []string{
				"image.png",   // File header
				"Binary file", // Binary indicator
			},
		},
		{
			name: "file with no hunks",
			files: []DiffFile{
				{
					NewPath: "empty.go",
					Hunks:   []DiffHunk{},
				},
			},
			width: 80,
			contains: []string{
				"empty.go",   // File header
				"No changes", // No hunks message
			},
		},
		{
			name: "renamed file",
			files: []DiffFile{
				{
					OldPath:    "old_name.go",
					NewPath:    "new_name.go",
					IsRenamed:  true,
					Similarity: 95,
					Additions:  2,
					Deletions:  1,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "old line"},
								{Type: LineAddition, NewLineNum: 1, Content: "new line"},
							},
						},
					},
				},
			},
			width: 80,
			contains: []string{
				"new_name.go", // Uses NewPath for renamed files
				"old line",
				"new line",
			},
		},
		{
			name: "new file",
			files: []DiffFile{
				{
					NewPath:   "new_file.go",
					OldPath:   "/dev/null",
					IsNew:     true,
					Additions: 15,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,2 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "package newpkg"},
								{Type: LineAddition, NewLineNum: 2, Content: "func NewFunc() {}"},
							},
						},
					},
				},
			},
			width: 80,
			contains: []string{
				"new_file.go",
				"package newpkg",
				"func NewFunc() {}",
			},
		},
		{
			name: "mixed file types",
			files: []DiffFile{
				{
					NewPath:   "added.go",
					IsNew:     true,
					Additions: 10,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "new content"},
							},
						},
					},
				},
				{
					NewPath:   "modified.go",
					Additions: 5,
					Deletions: 3,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "modified content"},
							},
						},
					},
				},
				{
					OldPath:   "deleted.go",
					IsDeleted: true,
					Deletions: 8,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +0,0 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "deleted content"},
							},
						},
					},
				},
				{
					NewPath:  "binary.png",
					IsBinary: true,
				},
			},
			width: 80,
			contains: []string{
				"added.go",
				"modified.go",
				"deleted.go",
				"binary.png",
				"new content",
				"modified content",
				"deleted content",
				"Binary file",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dirNode := createDirNodeWithFiles(tc.files)
			m := New().SetSize(tc.width, 50)

			result := m.renderDirectoryDiffContent(dirNode, tc.width)

			if tc.equals != "" {
				require.Equal(t, tc.equals, result)
			}

			for _, want := range tc.contains {
				require.Contains(t, result, want, "expected output to contain %q", want)
			}
		})
	}
}

// TestRenderDirectoryDiffContent_FileHeaderRendering tests that file headers
// include proper filename and stats
func TestRenderDirectoryDiffContent_FileHeaderRendering(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "test.go",
			Additions: 15,
			Deletions: 7,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines:  []DiffLine{{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "test"}},
				},
			},
		},
	}

	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)

	// Should contain file header with name and stats
	require.Contains(t, result, "test.go")
	require.Contains(t, result, "+15")
	require.Contains(t, result, "-7")
}

// TestRenderDirectoryDiffContent_DiffContentAggregation tests that diff content
// from multiple files is properly aggregated with separators
func TestRenderDirectoryDiffContent_DiffContentAggregation(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "first.go",
			Additions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "first file content"},
					},
				},
			},
		},
		{
			NewPath:   "second.go",
			Additions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "second file content"},
					},
				},
			},
		},
		{
			NewPath:   "third.go",
			Additions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "third file content"},
					},
				},
			},
		},
	}

	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)

	// All files should be present in order
	require.Contains(t, result, "first.go")
	require.Contains(t, result, "second.go")
	require.Contains(t, result, "third.go")
	require.Contains(t, result, "first file content")
	require.Contains(t, result, "second file content")
	require.Contains(t, result, "third file content")

	// Files should appear in order (first before second before third)
	firstIdx := strings.Index(result, "first.go")
	secondIdx := strings.Index(result, "second.go")
	thirdIdx := strings.Index(result, "third.go")
	require.Less(t, firstIdx, secondIdx)
	require.Less(t, secondIdx, thirdIdx)
}

// TestRenderDirectoryDiffContent_NestedDirectory tests rendering a directory
// that itself contains subdirectories with files
func TestRenderDirectoryDiffContent_NestedDirectory(t *testing.T) {
	// Create a nested structure: parent -> child dir -> files
	childDir := &FileTreeNode{
		Name:     "subdir",
		Path:     "parent/subdir",
		IsDir:    true,
		Expanded: true,
		Depth:    1,
		Children: []*FileTreeNode{
			{
				Name:  "nested.go",
				Path:  "parent/subdir/nested.go",
				IsDir: false,
				Depth: 2,
				File: &DiffFile{
					NewPath:   "nested.go",
					Additions: 5,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "nested content"},
							},
						},
					},
				},
			},
		},
	}

	parentDir := &FileTreeNode{
		Name:     "parent",
		Path:     "parent",
		IsDir:    true,
		Expanded: true,
		Depth:    0,
		Children: []*FileTreeNode{
			{
				Name:  "root.go",
				Path:  "parent/root.go",
				IsDir: false,
				Depth: 1,
				File: &DiffFile{
					NewPath:   "root.go",
					Additions: 3,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "root content"},
							},
						},
					},
				},
			},
			childDir,
		},
	}

	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(parentDir, 80)

	// Should include both root file and nested file content
	require.Contains(t, result, "root.go")
	require.Contains(t, result, "nested.go")
	require.Contains(t, result, "root content")
	require.Contains(t, result, "nested content")
}

// Golden tests for renderDirectoryDiffContent visual output

func TestView_Golden_DirectoryDiff_Empty(t *testing.T) {
	dirNode := createDirNodeWithFiles([]DiffFile{})
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_DirectoryDiff_SingleFile(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "main.go",
			Additions: 10,
			Deletions: 3,
			Hunks: []DiffHunk{
				{
					OldStart: 5,
					OldCount: 4,
					NewStart: 5,
					NewCount: 5,
					Header:   "@@ -5,4 +5,5 @@ package main",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 5, NewLineNum: 5, Content: "func main() {"},
						{Type: LineDeletion, OldLineNum: 6, NewLineNum: 0, Content: "\tfmt.Println(\"old\")"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 6, Content: "\tfmt.Println(\"new\")"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 7, Content: "\tfmt.Println(\"extra\")"},
						{Type: LineContext, OldLineNum: 7, NewLineNum: 8, Content: "}"},
					},
				},
			},
		},
	}
	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_DirectoryDiff_MultipleFiles(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "cmd/main.go",
			Additions: 15,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "import \"fmt\""},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "import ("},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "\t\"fmt\""},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 4, Content: ")"},
					},
				},
			},
		},
		{
			NewPath:   "pkg/utils.go",
			Additions: 20,
			Deletions: 0,
			IsNew:     true,
			Hunks: []DiffHunk{
				{
					Header: "@@ -0,0 +1,3 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "package utils"},
						{Type: LineAddition, NewLineNum: 2, Content: ""},
						{Type: LineAddition, NewLineNum: 3, Content: "func Helper() {}"},
					},
				},
			},
		},
		{
			OldPath:   "deprecated.go",
			IsDeleted: true,
			Deletions: 8,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +0,0 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineDeletion, OldLineNum: 1, Content: "package deprecated"},
						{Type: LineDeletion, OldLineNum: 2, Content: "// This is deprecated"},
					},
				},
			},
		},
	}
	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_DirectoryDiff_BinaryFile(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:  "assets/logo.png",
			IsBinary: true,
		},
	}
	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_DirectoryDiff_RenamedFile(t *testing.T) {
	files := []DiffFile{
		{
			OldPath:    "old_config.yaml",
			NewPath:    "new_config.yaml",
			IsRenamed:  true,
			Similarity: 92,
			Additions:  3,
			Deletions:  1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "version: 1"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "name: old"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "name: new"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "enabled: true"},
						{Type: LineContext, OldLineNum: 3, NewLineNum: 4, Content: "debug: false"},
					},
				},
			},
		},
	}
	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_DirectoryDiff_MixedTypes(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "added.go",
			IsNew:     true,
			Additions: 5,
			Hunks: []DiffHunk{
				{
					Header: "@@ -0,0 +1,2 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "package added"},
						{Type: LineAddition, NewLineNum: 2, Content: "// new file"},
					},
				},
			},
		},
		{
			NewPath:   "modified.go",
			Additions: 2,
			Deletions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +1,3 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package mod"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "// old"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "// new"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "// extra"},
					},
				},
			},
		},
		{
			OldPath:   "deleted.go",
			IsDeleted: true,
			Deletions: 3,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +0,0 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineDeletion, OldLineNum: 1, Content: "package deleted"},
						{Type: LineDeletion, OldLineNum: 2, Content: "// gone"},
					},
				},
			},
		},
		{
			NewPath:  "binary.dat",
			IsBinary: true,
		},
		{
			OldPath:    "renamed_old.txt",
			NewPath:    "renamed_new.txt",
			IsRenamed:  true,
			Similarity: 100,
			Hunks:      []DiffHunk{},
		},
	}
	dirNode := createDirNodeWithFiles(files)
	m := New().SetSize(80, 50)
	result := m.renderDirectoryDiffContent(dirNode, 80)
	teatest.RequireEqualOutput(t, []byte(result))
}

// =============================================================================
// renderFullCommitDiffContent Tests
// =============================================================================

// Helper to create a model with commit preview state set up for testing
func setupModelWithPreviewCommit(t *testing.T, files []DiffFile, commit *git.CommitInfo) Model {
	mockClock := mocks.NewMockClock(t)
	// Set a fixed time for deterministic output
	fixedTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	mockClock.EXPECT().Now().Return(fixedTime).Maybe()

	m := New().SetSize(80, 50).SetClock(mockClock)
	m.previewCommitFiles = files
	m.previewCommitLoading = false
	if commit != nil {
		m.commits = []git.CommitInfo{*commit}
		m.selectedCommit = 0
		m.previewCommitHash = commit.Hash // Must match selected commit for render to work
	}
	return m
}

// TestRenderFullCommitDiffContent tests the renderFullCommitDiffContent function
func TestRenderFullCommitDiffContent(t *testing.T) {
	tests := []struct {
		name     string
		files    []DiffFile
		commit   *git.CommitInfo
		loading  bool
		width    int
		contains []string
		equals   string // If non-empty, check exact match
	}{
		{
			name:     "no commit selected",
			files:    []DiffFile{},
			commit:   nil,
			width:    80,
			contains: []string{"No changes to display"},
		},
		{
			name:     "empty commit - no files, not loading",
			files:    []DiffFile{},
			commit:   &git.CommitInfo{Hash: "abc123", Author: "Test", Subject: "Empty commit", Date: time.Now()},
			width:    80,
			contains: []string{"No changes to display"},
		},
		{
			name:     "loading state",
			files:    []DiffFile{},
			commit:   &git.CommitInfo{Hash: "abc123", Author: "Test", Subject: "Loading", Date: time.Now()},
			loading:  true,
			width:    80,
			contains: []string{"Loading"},
		},
		{
			name: "single file with commit",
			files: []DiffFile{
				{
					NewPath:   "main.go",
					Additions: 5,
					Deletions: 2,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,2 +1,3 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
								{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "// old comment"},
								{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "// new comment"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{Hash: "file123", Author: "Test", Subject: "File commit", Date: time.Now()},
			width:  80,
			contains: []string{
				"main.go",        // File header
				"+5",             // Additions stat
				"-2",             // Deletions stat
				"package main",   // Context content
				"// old comment", // Deleted content
				"// new comment", // Added content
			},
		},
		{
			name: "single file with commit header",
			files: []DiffFile{
				{
					NewPath:   "test.go",
					Additions: 3,
					Deletions: 1,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,2 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package test"},
								{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "func TestFunc() {}"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{
				Hash:    "abc1234567890def",
				Author:  "Test Author",
				Subject: "Add test function",
				Date:    time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			},
			width: 80,
			contains: []string{
				"commit abc1234567890def", // Commit hash line
				"Author: Test Author",     // Author line
				"Date:",                   // Date line
				"Add test function",       // Commit subject
				"test.go",                 // File header
				"package test",            // File content
				"func TestFunc() {}",      // Added line
			},
		},
		{
			name: "multiple files with commit header",
			files: []DiffFile{
				{
					NewPath:   "file1.go",
					Additions: 10,
					Deletions: 3,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "first file"},
							},
						},
					},
				},
				{
					NewPath:   "file2.go",
					Additions: 5,
					Deletions: 1,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "second file"},
							},
						},
					},
				},
				{
					NewPath:   "file3.go",
					Additions: 2,
					Deletions: 0,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "third file"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{
				Hash:    "def456789abc",
				Author:  "Multi File Author",
				Subject: "Update multiple files",
				Date:    time.Date(2025, 1, 14, 14, 0, 0, 0, time.UTC),
			},
			width: 80,
			contains: []string{
				"commit def456789abc",       // Commit hash
				"Author: Multi File Author", // Author
				"Update multiple files",     // Subject
				"file1.go",                  // First file header
				"file2.go",                  // Second file header
				"file3.go",                  // Third file header
				"first file",                // First file content
				"second file",               // Second file content
				"third file",                // Third file content
			},
		},
		{
			name: "binary file",
			files: []DiffFile{
				{
					NewPath:  "image.png",
					IsBinary: true,
				},
			},
			commit: &git.CommitInfo{Hash: "binary123", Author: "Test", Subject: "Binary", Date: time.Now()},
			width:  80,
			contains: []string{
				"image.png",   // File header
				"Binary file", // Binary indicator
			},
		},
		{
			name: "deleted file uses OldPath",
			files: []DiffFile{
				{
					OldPath:   "removed_file.go",
					NewPath:   "/dev/null",
					IsDeleted: true,
					Deletions: 10,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,2 +0,0 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "package removed"},
								{Type: LineDeletion, OldLineNum: 2, Content: "// goodbye"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{Hash: "deleted123", Author: "Test", Subject: "Deleted", Date: time.Now()},
			width:  80,
			contains: []string{
				"removed_file.go", // Should use OldPath
				"package removed", // Deleted content
				"// goodbye",      // Deleted content
			},
		},
		{
			name: "file with no hunks",
			files: []DiffFile{
				{
					NewPath: "empty_diff.go",
					Hunks:   []DiffHunk{},
				},
			},
			commit: &git.CommitInfo{Hash: "nohunks123", Author: "Test", Subject: "No hunks", Date: time.Now()},
			width:  80,
			contains: []string{
				"empty_diff.go", // File header
				"No changes",    // No hunks message
			},
		},
		{
			name: "renamed file",
			files: []DiffFile{
				{
					OldPath:    "old_name.go",
					NewPath:    "new_name.go",
					IsRenamed:  true,
					Similarity: 95,
					Additions:  2,
					Deletions:  1,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "old line"},
								{Type: LineAddition, NewLineNum: 1, Content: "new line"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{Hash: "renamed123", Author: "Test", Subject: "Renamed", Date: time.Now()},
			width:  80,
			contains: []string{
				"new_name.go", // Uses NewPath for renamed files
				"old line",
				"new line",
			},
		},
		{
			name: "new file",
			files: []DiffFile{
				{
					NewPath:   "new_file.go",
					OldPath:   "/dev/null",
					IsNew:     true,
					Additions: 15,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,2 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "package newpkg"},
								{Type: LineAddition, NewLineNum: 2, Content: "func NewFunc() {}"},
							},
						},
					},
				},
			},
			commit: &git.CommitInfo{Hash: "newfile123", Author: "Test", Subject: "New file", Date: time.Now()},
			width:  80,
			contains: []string{
				"new_file.go",
				"package newpkg",
				"func NewFunc() {}",
			},
		},
		{
			name: "mixed file types with commit header",
			files: []DiffFile{
				{
					NewPath:   "added.go",
					IsNew:     true,
					Additions: 10,
					Hunks: []DiffHunk{
						{
							Header: "@@ -0,0 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineAddition, NewLineNum: 1, Content: "new content"},
							},
						},
					},
				},
				{
					NewPath:   "modified.go",
					Additions: 5,
					Deletions: 3,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +1,1 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "modified content"},
							},
						},
					},
				},
				{
					OldPath:   "deleted.go",
					IsDeleted: true,
					Deletions: 8,
					Hunks: []DiffHunk{
						{
							Header: "@@ -1,1 +0,0 @@",
							Lines: []DiffLine{
								{Type: LineHunkHeader, Content: ""},
								{Type: LineDeletion, OldLineNum: 1, Content: "deleted content"},
							},
						},
					},
				},
				{
					NewPath:  "binary.png",
					IsBinary: true,
				},
			},
			commit: &git.CommitInfo{
				Hash:    "mixed123456",
				Author:  "Mixed Author",
				Subject: "Mixed changes commit",
				Date:    time.Date(2025, 1, 10, 8, 0, 0, 0, time.UTC),
			},
			width: 80,
			contains: []string{
				"commit mixed123456",
				"Author: Mixed Author",
				"Mixed changes commit",
				"added.go",
				"modified.go",
				"deleted.go",
				"binary.png",
				"new content",
				"modified content",
				"deleted content",
				"Binary file",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := setupModelWithPreviewCommit(t, tc.files, tc.commit)
			if tc.loading {
				m.previewCommitLoading = true
				m.previewCommitFiles = nil
			}

			result := m.renderFullCommitDiff(tc.width, 50)

			if tc.equals != "" {
				require.Equal(t, tc.equals, result)
			}

			for _, want := range tc.contains {
				require.Contains(t, result, want, "expected output to contain %q", want)
			}
		})
	}
}

// TestRenderFullCommitDiffContent_CommitHeaderRendering tests that commit headers
// include proper hash, author, date, and subject
func TestRenderFullCommitDiffContent_CommitHeaderRendering(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "test.go",
			Additions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines:  []DiffLine{{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "test"}},
				},
			},
		},
	}
	commit := &git.CommitInfo{
		Hash:    "1234567890abcdef1234567890abcdef12345678",
		Author:  "John Doe <john@example.com>",
		Subject: "feat: Add new feature with detailed description",
		Date:    time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)

	// Verify commit hash line format
	require.Contains(t, result, "commit 1234567890abcdef1234567890abcdef12345678")

	// Verify author line format
	require.Contains(t, result, "Author: John Doe <john@example.com>")

	// Verify date line format (includes relative time)
	require.Contains(t, result, "Date:")
	require.Contains(t, result, "2025") // Year should be in date

	// Verify subject is indented (like git log format)
	require.Contains(t, result, "feat: Add new feature with detailed description")

	// Verify header appears before file content
	headerIdx := strings.Index(result, "commit 1234567890abcdef")
	fileIdx := strings.Index(result, "test.go")
	require.Less(t, headerIdx, fileIdx, "commit header should appear before file content")
}

// TestRenderFullCommitDiffContent_FileHeaderRendering tests that file headers
// within commit diff are properly rendered
func TestRenderFullCommitDiffContent_FileHeaderRendering(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "internal/pkg/handler.go",
			Additions: 25,
			Deletions: 10,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package pkg"},
					},
				},
			},
		},
	}
	commit := &git.CommitInfo{Hash: "fileheader123", Author: "Test", Subject: "File headers", Date: time.Now()}

	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)

	// File header should include filename
	require.Contains(t, result, "internal/pkg/handler.go")

	// File header should include stats
	require.Contains(t, result, "+25")
	require.Contains(t, result, "-10")
}

// TestRenderFullCommitDiffContent_NoFilesShowsNoChanges tests that empty commit
// shows empty state message
func TestRenderFullCommitDiffContent_NoFilesShowsNoChanges(t *testing.T) {
	commit := &git.CommitInfo{Hash: "empty123", Author: "Test", Subject: "Empty", Date: time.Now()}
	m := setupModelWithPreviewCommit(t, []DiffFile{}, commit)
	result := m.renderFullCommitDiff(80, 50)

	require.Contains(t, result, "No changes to display")
}

// TestRenderFullCommitDiffContent_LoadingState tests that loading state
// shows loading message
func TestRenderFullCommitDiffContent_LoadingState(t *testing.T) {
	m := New().SetSize(80, 50)
	m.commits = []git.CommitInfo{{Hash: "loading123", Author: "Test", Subject: "Loading", Date: time.Now()}}
	m.selectedCommit = 0
	m.previewCommitHash = "loading123"
	m.previewCommitFiles = nil
	m.previewCommitLoading = true

	result := m.renderFullCommitDiff(80, 50)

	require.Contains(t, result, "Loading")
}

// TestRenderFullCommitDiffContent_SelectedCommitOutOfBounds tests edge case
// where selectedCommit index is invalid
func TestRenderFullCommitDiffContent_SelectedCommitOutOfBounds(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "test.go",
			Additions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines:  []DiffLine{{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "test"}},
				},
			},
		},
	}

	m := setupModelWithPreviewCommit(t, files, nil)
	// Set up invalid selectedCommit index
	m.commits = []git.CommitInfo{{Hash: "abc"}}
	m.selectedCommit = 5 // Out of bounds

	result := m.renderFullCommitDiff(80, 50)

	// Should show empty state since selectedCommit is out of bounds
	require.Contains(t, result, "No changes to display")
}

// Golden tests for renderFullCommitDiffContent visual output

func TestView_Golden_FullCommitDiff_Empty(t *testing.T) {
	commit := &git.CommitInfo{Hash: "empty123", Author: "Test", Subject: "Empty", Date: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)}
	m := setupModelWithPreviewCommit(t, []DiffFile{}, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_Loading(t *testing.T) {
	m := New().SetSize(80, 50)
	m.commits = []git.CommitInfo{{Hash: "loading123", Author: "Test", Subject: "Loading", Date: time.Now()}}
	m.selectedCommit = 0
	m.previewCommitHash = "loading123"
	m.previewCommitFiles = nil
	m.previewCommitLoading = true
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_SingleFileNoHeader(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "main.go",
			Additions: 10,
			Deletions: 3,
			Hunks: []DiffHunk{
				{
					OldStart: 5,
					OldCount: 4,
					NewStart: 5,
					NewCount: 5,
					Header:   "@@ -5,4 +5,5 @@ package main",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: "package main"},
						{Type: LineContext, OldLineNum: 5, NewLineNum: 5, Content: "func main() {"},
						{Type: LineDeletion, OldLineNum: 6, NewLineNum: 0, Content: "\tfmt.Println(\"old\")"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 6, Content: "\tfmt.Println(\"new\")"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 7, Content: "\tfmt.Println(\"extra\")"},
						{Type: LineContext, OldLineNum: 7, NewLineNum: 8, Content: "}"},
					},
				},
			},
		},
	}
	commit := &git.CommitInfo{Hash: "single123", Author: "Test", Subject: "Single file", Date: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)}
	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_SingleFileWithHeader(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "handler.go",
			Additions: 15,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package handler"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "func Old() {}"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "func New() error {"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "\treturn nil"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 4, Content: "}"},
					},
				},
			},
		},
	}
	commit := &git.CommitInfo{
		Hash:    "abc123def456789",
		Author:  "Test Author <test@example.com>",
		Subject: "refactor: Update handler function",
		Date:    time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_MultipleFiles(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "cmd/main.go",
			Additions: 15,
			Deletions: 5,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,3 +1,4 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "import \"fmt\""},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "import ("},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "\t\"fmt\""},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 4, Content: ")"},
					},
				},
			},
		},
		{
			NewPath:   "pkg/utils.go",
			Additions: 20,
			Deletions: 0,
			IsNew:     true,
			Hunks: []DiffHunk{
				{
					Header: "@@ -0,0 +1,3 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "package utils"},
						{Type: LineAddition, NewLineNum: 2, Content: ""},
						{Type: LineAddition, NewLineNum: 3, Content: "func Helper() {}"},
					},
				},
			},
		},
		{
			OldPath:   "deprecated.go",
			IsDeleted: true,
			Deletions: 8,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +0,0 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineDeletion, OldLineNum: 1, Content: "package deprecated"},
						{Type: LineDeletion, OldLineNum: 2, Content: "// This is deprecated"},
					},
				},
			},
		},
	}
	commit := &git.CommitInfo{
		Hash:    "def456789abc123",
		Author:  "Developer <dev@example.com>",
		Subject: "feat: Add utils package and update imports",
		Date:    time.Date(2025, 1, 14, 14, 0, 0, 0, time.UTC),
	}
	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_BinaryFile(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:  "assets/logo.png",
			IsBinary: true,
		},
	}
	commit := &git.CommitInfo{
		Hash:    "binary123",
		Author:  "Designer",
		Subject: "Add logo asset",
		Date:    time.Date(2025, 1, 13, 16, 0, 0, 0, time.UTC),
	}
	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_FullCommitDiff_MixedTypes(t *testing.T) {
	files := []DiffFile{
		{
			NewPath:   "added.go",
			IsNew:     true,
			Additions: 5,
			Hunks: []DiffHunk{
				{
					Header: "@@ -0,0 +1,2 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineAddition, NewLineNum: 1, Content: "package added"},
						{Type: LineAddition, NewLineNum: 2, Content: "// new file"},
					},
				},
			},
		},
		{
			NewPath:   "modified.go",
			Additions: 2,
			Deletions: 1,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +1,3 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package mod"},
						{Type: LineDeletion, OldLineNum: 2, NewLineNum: 0, Content: "// old"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 2, Content: "// new"},
						{Type: LineAddition, OldLineNum: 0, NewLineNum: 3, Content: "// extra"},
					},
				},
			},
		},
		{
			OldPath:   "deleted.go",
			IsDeleted: true,
			Deletions: 3,
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,2 +0,0 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader, Content: ""},
						{Type: LineDeletion, OldLineNum: 1, Content: "package deleted"},
						{Type: LineDeletion, OldLineNum: 2, Content: "// gone"},
					},
				},
			},
		},
		{
			NewPath:  "binary.dat",
			IsBinary: true,
		},
		{
			OldPath:    "renamed_old.txt",
			NewPath:    "renamed_new.txt",
			IsRenamed:  true,
			Similarity: 100,
			Hunks:      []DiffHunk{},
		},
	}
	commit := &git.CommitInfo{
		Hash:    "mixed999abc",
		Author:  "Multi Author",
		Subject: "Mixed changes: add, modify, delete, binary, rename",
		Date:    time.Date(2025, 1, 12, 9, 0, 0, 0, time.UTC),
	}
	m := setupModelWithPreviewCommit(t, files, commit)
	result := m.renderFullCommitDiff(80, 50)
	teatest.RequireEqualOutput(t, []byte(result))
}

// =============================================================================
// renderBranchList Tests
// =============================================================================

func TestRenderBranchListItem_BasicBranch(t *testing.T) {
	branch := git.BranchInfo{Name: "feature/auth", IsCurrent: false}
	result := renderBranchListItem(branch, false, false, 40)

	require.NotEmpty(t, result)
	require.Contains(t, result, "feature/auth")
	// Should start with the branch name (no indicator prefix)
	require.True(t, strings.HasPrefix(result, "feature/auth"), "expected branch name at start")
}

func TestRenderBranchListItem_CurrentBranch(t *testing.T) {
	branch := git.BranchInfo{Name: "main", IsCurrent: true}
	result := renderBranchListItem(branch, false, false, 40)

	require.Contains(t, result, "main")
	require.Contains(t, result, "(current)")
}

func TestRenderBranchListItem_SelectedBranch(t *testing.T) {
	branch := git.BranchInfo{Name: "develop", IsCurrent: false}
	result := renderBranchListItem(branch, true, false, 40)

	require.Contains(t, result, "develop")
	require.NotContains(t, result, ">") // No > indicator, selection shown via highlight
}

func TestRenderBranchListItem_SelectedAndFocused(t *testing.T) {
	branch := git.BranchInfo{Name: "main", IsCurrent: true}
	result := renderBranchListItem(branch, true, true, 50)

	require.Contains(t, result, "main")
	require.Contains(t, result, "(current)")
	require.NotContains(t, result, ">") // No > indicator, selection shown via highlight
}

func TestRenderBranchListItem_Truncation(t *testing.T) {
	branch := git.BranchInfo{Name: "feature/very-long-branch-name-that-should-be-truncated", IsCurrent: false}
	result := renderBranchListItem(branch, false, false, 20)

	// Should not exceed width
	require.LessOrEqual(t, lipgloss.Width(result), 20)
}

func TestRenderBranchListItem_ZeroWidth(t *testing.T) {
	branch := git.BranchInfo{Name: "main", IsCurrent: false}
	result := renderBranchListItem(branch, false, false, 0)

	require.Empty(t, result)
}

func TestRenderBranchList_EmptyList(t *testing.T) {
	result := renderBranchList([]git.BranchInfo{}, 0, 0, 40, 10, true)
	require.Contains(t, result, "No branches")
}

func TestRenderBranchList_SingleBranch(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
	}
	result := renderBranchList(branches, 0, 0, 40, 10, true)

	require.Contains(t, result, "main")
	require.Contains(t, result, "(current)")
}

func TestRenderBranchList_MultipleBranches(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	result := renderBranchList(branches, 0, 0, 40, 10, true)

	require.Contains(t, result, "main")
	require.Contains(t, result, "develop")
	require.Contains(t, result, "feature/auth")
}

func TestRenderBranchList_ScrollState(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "branch-1", IsCurrent: false},
		{Name: "branch-2", IsCurrent: false},
		{Name: "branch-3", IsCurrent: false},
		{Name: "branch-4", IsCurrent: false},
		{Name: "branch-5", IsCurrent: false},
	}
	// Scroll down by 2, height of 2 - should show branch-3 and branch-4
	result := renderBranchList(branches, 2, 2, 40, 2, true)

	require.Contains(t, result, "branch-3")
	require.Contains(t, result, "branch-4")
	require.NotContains(t, result, "branch-1")
	require.NotContains(t, result, "branch-2")
}

func TestRenderBranchList_ZeroDimensions(t *testing.T) {
	branches := []git.BranchInfo{{Name: "main", IsCurrent: true}}

	// Zero width
	result := renderBranchList(branches, 0, 0, 0, 10, true)
	require.Empty(t, result)

	// Zero height
	result = renderBranchList(branches, 0, 0, 40, 0, true)
	require.Empty(t, result)
}

func TestRenderBranchList_HeightPadding(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
	}
	result := renderBranchList(branches, 0, 0, 40, 5, true)

	// Should have 5 lines (1 branch + 4 padding)
	lines := strings.Split(result, "\n")
	require.Equal(t, 5, len(lines))
}

// Golden tests for renderBranchList

func TestView_Golden_BranchList_Empty(t *testing.T) {
	result := renderBranchList([]git.BranchInfo{}, 0, 0, 40, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_BranchList_SingleCurrent(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
	}
	result := renderBranchList(branches, 0, 0, 40, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_BranchList_Multiple(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
		{Name: "feature/profile", IsCurrent: false},
		{Name: "bugfix/login-issue", IsCurrent: false},
	}
	result := renderBranchList(branches, 0, 0, 50, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_BranchList_SelectedFocused(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	// Select index 1 (develop), focused
	result := renderBranchList(branches, 1, 0, 50, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_BranchList_SelectedUnfocused(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
		{Name: "feature/auth", IsCurrent: false},
	}
	// Select index 1 (develop), unfocused
	result := renderBranchList(branches, 1, 0, 50, 10, false)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_BranchList_CurrentSelected(t *testing.T) {
	branches := []git.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "develop", IsCurrent: false},
	}
	// Select the current branch (index 0), focused
	result := renderBranchList(branches, 0, 0, 50, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

// =============================================================================
// renderWorktreeList Tests
// =============================================================================

func TestRenderWorktreeListItem_BasicWorktree(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/path/to/my-worktree", Branch: "feature/auth", HEAD: "abc123"}
	result := renderWorktreeListItem(worktree, false, false, 50)

	require.NotEmpty(t, result)
	require.Contains(t, result, "my-worktree")
	require.Contains(t, result, "(feature/auth)")
	// Should start with the worktree name (no indicator prefix)
	require.True(t, strings.HasPrefix(result, "my-worktree"), "expected worktree name at start")
}

func TestRenderWorktreeListItem_SelectedWorktree(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/path/to/my-worktree", Branch: "main", HEAD: "def456"}
	result := renderWorktreeListItem(worktree, true, false, 50)

	require.Contains(t, result, "my-worktree")
	require.NotContains(t, result, ">") // No > indicator, selection shown via highlight
}

func TestRenderWorktreeListItem_SelectedAndFocused(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/path/to/worktree", Branch: "develop", HEAD: "ghi789"}
	result := renderWorktreeListItem(worktree, true, true, 50)

	require.Contains(t, result, "worktree")
	require.Contains(t, result, "(develop)")
	require.NotContains(t, result, ">") // No > indicator, selection shown via highlight
}

func TestRenderWorktreeListItem_NoBranch(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/path/to/detached", Branch: "", HEAD: "abc123"}
	result := renderWorktreeListItem(worktree, false, false, 50)

	require.Contains(t, result, "detached")
	// Should not have parentheses with empty branch
	require.NotContains(t, result, "()")
}

func TestRenderWorktreeListItem_Truncation(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/very/long/path/to/my-very-long-worktree-name", Branch: "feature/auth", HEAD: "abc123"}
	result := renderWorktreeListItem(worktree, false, false, 30)

	// Should not exceed width
	require.LessOrEqual(t, lipgloss.Width(result), 30)
}

func TestRenderWorktreeListItem_ZeroWidth(t *testing.T) {
	worktree := git.WorktreeInfo{Path: "/path/to/wt", Branch: "main", HEAD: "abc123"}
	result := renderWorktreeListItem(worktree, false, false, 0)

	require.Empty(t, result)
}

func TestRenderWorktreeList_EmptyList(t *testing.T) {
	result := renderWorktreeList([]git.WorktreeInfo{}, 0, 0, 50, 10, true)
	require.Contains(t, result, "No worktrees")
}

func TestRenderWorktreeList_SingleWorktree(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123"},
	}
	result := renderWorktreeList(worktrees, 0, 0, 50, 10, true)

	require.Contains(t, result, "project")
	require.Contains(t, result, "(main)")
}

func TestRenderWorktreeList_MultipleWorktrees(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123"},
		{Path: "/home/user/project-feature", Branch: "feature/auth", HEAD: "def456"},
		{Path: "/home/user/project-bugfix", Branch: "bugfix/issue-42", HEAD: "ghi789"},
	}
	result := renderWorktreeList(worktrees, 0, 0, 50, 10, true)

	require.Contains(t, result, "project")
	require.Contains(t, result, "project-feature")
	require.Contains(t, result, "project-bugfix")
	require.Contains(t, result, "(main)")
	require.Contains(t, result, "(feature/auth)")
	require.Contains(t, result, "(bugfix/issue-42)")
}

func TestRenderWorktreeList_ScrollState(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/wt/wt-1", Branch: "branch-1", HEAD: "a"},
		{Path: "/wt/wt-2", Branch: "branch-2", HEAD: "b"},
		{Path: "/wt/wt-3", Branch: "branch-3", HEAD: "c"},
		{Path: "/wt/wt-4", Branch: "branch-4", HEAD: "d"},
		{Path: "/wt/wt-5", Branch: "branch-5", HEAD: "e"},
	}
	// Scroll down by 2, height of 2 - should show wt-3 and wt-4
	result := renderWorktreeList(worktrees, 2, 2, 50, 2, true)

	require.Contains(t, result, "wt-3")
	require.Contains(t, result, "wt-4")
	require.NotContains(t, result, "wt-1")
	require.NotContains(t, result, "wt-2")
}

func TestRenderWorktreeList_ZeroDimensions(t *testing.T) {
	worktrees := []git.WorktreeInfo{{Path: "/path/wt", Branch: "main", HEAD: "abc"}}

	// Zero width
	result := renderWorktreeList(worktrees, 0, 0, 0, 10, true)
	require.Empty(t, result)

	// Zero height
	result = renderWorktreeList(worktrees, 0, 0, 50, 0, true)
	require.Empty(t, result)
}

func TestRenderWorktreeList_HeightPadding(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/path/wt", Branch: "main", HEAD: "abc"},
	}
	result := renderWorktreeList(worktrees, 0, 0, 50, 5, true)

	// Should have 5 lines (1 worktree + 4 padding)
	lines := strings.Split(result, "\n")
	require.Equal(t, 5, len(lines))
}

// Golden tests for renderWorktreeList

func TestView_Golden_WorktreeList_Empty(t *testing.T) {
	result := renderWorktreeList([]git.WorktreeInfo{}, 0, 0, 50, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_WorktreeList_Single(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123def"},
	}
	result := renderWorktreeList(worktrees, 0, 0, 50, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_WorktreeList_Multiple(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123"},
		{Path: "/home/user/project-feature", Branch: "feature/auth", HEAD: "def456"},
		{Path: "/home/user/project-bugfix", Branch: "bugfix/issue-42", HEAD: "ghi789"},
	}
	result := renderWorktreeList(worktrees, 0, 0, 60, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_WorktreeList_SelectedFocused(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123"},
		{Path: "/home/user/project-feature", Branch: "feature/auth", HEAD: "def456"},
		{Path: "/home/user/project-bugfix", Branch: "bugfix/issue-42", HEAD: "ghi789"},
	}
	// Select index 1 (project-feature), focused
	result := renderWorktreeList(worktrees, 1, 0, 60, 10, true)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestView_Golden_WorktreeList_SelectedUnfocused(t *testing.T) {
	worktrees := []git.WorktreeInfo{
		{Path: "/home/user/project", Branch: "main", HEAD: "abc123"},
		{Path: "/home/user/project-feature", Branch: "feature/auth", HEAD: "def456"},
		{Path: "/home/user/project-bugfix", Branch: "bugfix/issue-42", HEAD: "ghi789"},
	}
	// Select index 1 (project-feature), unfocused
	result := renderWorktreeList(worktrees, 1, 0, 60, 10, false)
	teatest.RequireEqualOutput(t, []byte(result))
}
