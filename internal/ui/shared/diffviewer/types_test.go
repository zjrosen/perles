package diffviewer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Tests for ViewMode

func TestViewMode_Constants(t *testing.T) {
	// Verify enum values are sequential starting from 0
	require.Equal(t, ViewMode(0), ViewModeUnified, "ViewModeUnified should be 0")
	require.Equal(t, ViewMode(1), ViewModeSideBySide, "ViewModeSideBySide should be 1")
}

func TestViewMode_String(t *testing.T) {
	tests := []struct {
		mode     ViewMode
		expected string
	}{
		{ViewModeUnified, "UNIFIED"},
		{ViewModeSideBySide, "SIDE-BY-SIDE"},
		{ViewMode(99), "UNKNOWN"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.mode.String())
		})
	}
}

func TestLineType_Constants(t *testing.T) {
	// Verify enum values are sequential starting from 0
	require.Equal(t, LineType(0), LineContext, "LineContext should be 0")
	require.Equal(t, LineType(1), LineAddition, "LineAddition should be 1")
	require.Equal(t, LineType(2), LineDeletion, "LineDeletion should be 2")
	require.Equal(t, LineType(3), LineHunkHeader, "LineHunkHeader should be 3")
}

func TestDiffLine_ZeroValue(t *testing.T) {
	var line DiffLine

	// Zero value should be sensible defaults
	require.Equal(t, LineContext, line.Type, "zero Type should be LineContext")
	require.Equal(t, 0, line.OldLineNum, "zero OldLineNum should be 0")
	require.Equal(t, 0, line.NewLineNum, "zero NewLineNum should be 0")
	require.Equal(t, "", line.Content, "zero Content should be empty string")
}

func TestDiffHunk_ZeroValue(t *testing.T) {
	var hunk DiffHunk

	// Zero value should be sensible defaults
	require.Equal(t, 0, hunk.OldStart, "zero OldStart should be 0")
	require.Equal(t, 0, hunk.OldCount, "zero OldCount should be 0")
	require.Equal(t, 0, hunk.NewStart, "zero NewStart should be 0")
	require.Equal(t, 0, hunk.NewCount, "zero NewCount should be 0")
	require.Equal(t, "", hunk.Header, "zero Header should be empty string")
	require.Nil(t, hunk.Lines, "zero Lines should be nil")
}

func TestDiffFile_ZeroValue(t *testing.T) {
	var file DiffFile

	// Zero value should be sensible defaults
	require.Equal(t, "", file.OldPath, "zero OldPath should be empty string")
	require.Equal(t, "", file.NewPath, "zero NewPath should be empty string")
	require.Equal(t, 0, file.Additions, "zero Additions should be 0")
	require.Equal(t, 0, file.Deletions, "zero Deletions should be 0")
	require.False(t, file.IsBinary, "zero IsBinary should be false")
	require.False(t, file.IsRenamed, "zero IsRenamed should be false")
	require.False(t, file.IsNew, "zero IsNew should be false")
	require.False(t, file.IsDeleted, "zero IsDeleted should be false")
	require.Equal(t, 0, file.Similarity, "zero Similarity should be 0")
	require.Nil(t, file.Hunks, "zero Hunks should be nil")
}

func TestTypes_Exported(t *testing.T) {
	// This test verifies that all types are properly exported by constructing
	// instances with all fields set. If any field were unexported, this would
	// fail to compile.

	_ = DiffLine{
		Type:       LineAddition,
		OldLineNum: 10,
		NewLineNum: 12,
		Content:    "added line",
	}

	_ = DiffHunk{
		OldStart: 1,
		OldCount: 5,
		NewStart: 1,
		NewCount: 7,
		Header:   "@@ -1,5 +1,7 @@",
		Lines: []DiffLine{
			{Type: LineContext, Content: "context"},
		},
	}

	_ = DiffFile{
		OldPath:    "old/path.go",
		NewPath:    "new/path.go",
		Additions:  10,
		Deletions:  5,
		IsBinary:   false,
		IsRenamed:  true,
		IsNew:      false,
		IsDeleted:  false,
		Similarity: 95,
		Hunks:      []DiffHunk{},
	}
}

// Tests for DiffError and ErrorCategory

func TestErrorCategory_Constants(t *testing.T) {
	// Verify enum values are sequential starting from 0
	require.Equal(t, ErrorCategory(0), ErrCategoryParse, "ErrCategoryParse should be 0")
	require.Equal(t, ErrorCategory(1), ErrCategoryGitOp, "ErrCategoryGitOp should be 1")
	require.Equal(t, ErrorCategory(2), ErrCategoryPermission, "ErrCategoryPermission should be 2")
	require.Equal(t, ErrorCategory(3), ErrCategoryConflict, "ErrCategoryConflict should be 3")
	require.Equal(t, ErrorCategory(4), ErrCategoryTimeout, "ErrCategoryTimeout should be 4")
}

func TestErrorCategory_String(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		expected string
	}{
		{ErrCategoryParse, "Parse Error"},
		{ErrCategoryGitOp, "Git Operation Error"},
		{ErrCategoryPermission, "Permission Error"},
		{ErrCategoryConflict, "Conflict Error"},
		{ErrCategoryTimeout, "Timeout Error"},
		{ErrorCategory(99), "Unknown Error"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.category.String())
		})
	}
}

func TestDiffError_Error(t *testing.T) {
	err := DiffError{
		Category: ErrCategoryGitOp,
		Message:  "test error message",
	}

	// Should implement error interface
	var e error = err
	require.Equal(t, "test error message", e.Error())
}

func TestNewDiffError(t *testing.T) {
	err := NewDiffError(ErrCategoryGitOp, "git failed")
	require.Equal(t, ErrCategoryGitOp, err.Category)
	require.Equal(t, "git failed", err.Message)
}

func TestDiffError_WithHelpText(t *testing.T) {
	err := NewDiffError(ErrCategoryGitOp, "git failed")
	require.Empty(t, err.HelpText)

	err = err.WithHelpText("Try checking your git config")
	require.Equal(t, "Try checking your git config", err.HelpText)
}

func TestDiffError_Chaining(t *testing.T) {
	// Test fluent API chaining
	err := NewDiffError(ErrCategoryTimeout, "timed out").
		WithHelpText("This may happen with large repos")

	require.Equal(t, ErrCategoryTimeout, err.Category)
	require.Equal(t, "timed out", err.Message)
	require.Equal(t, "This may happen with large repos", err.HelpText)
}

// Tests for alignedPair

func TestAlignedPair_ZeroValue(t *testing.T) {
	var pair alignedPair

	// Zero value should have nil pointers
	require.Nil(t, pair.Left, "zero Left should be nil")
	require.Nil(t, pair.Right, "zero Right should be nil")
}

func TestAlignedPair_IsContext(t *testing.T) {
	contextLine := DiffLine{Type: LineContext, Content: "unchanged"}

	tests := []struct {
		name     string
		pair     alignedPair
		expected bool
	}{
		{
			name:     "both context lines",
			pair:     alignedPair{Left: &contextLine, Right: &contextLine},
			expected: true,
		},
		{
			name:     "left only",
			pair:     alignedPair{Left: &contextLine, Right: nil},
			expected: false,
		},
		{
			name:     "right only",
			pair:     alignedPair{Left: nil, Right: &contextLine},
			expected: false,
		},
		{
			name:     "both nil",
			pair:     alignedPair{Left: nil, Right: nil},
			expected: false,
		},
		{
			name: "left deletion, right addition",
			pair: alignedPair{
				Left:  &DiffLine{Type: LineDeletion},
				Right: &DiffLine{Type: LineAddition},
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pair.IsContext())
		})
	}
}

func TestAlignedPair_IsDeletion(t *testing.T) {
	deletionLine := DiffLine{Type: LineDeletion, Content: "deleted"}
	contextLine := DiffLine{Type: LineContext, Content: "context"}

	tests := []struct {
		name     string
		pair     alignedPair
		expected bool
	}{
		{
			name:     "deletion with nil right",
			pair:     alignedPair{Left: &deletionLine, Right: nil},
			expected: true,
		},
		{
			name:     "deletion with non-nil right",
			pair:     alignedPair{Left: &deletionLine, Right: &contextLine},
			expected: false,
		},
		{
			name:     "context with nil right",
			pair:     alignedPair{Left: &contextLine, Right: nil},
			expected: false,
		},
		{
			name:     "nil left",
			pair:     alignedPair{Left: nil, Right: &deletionLine},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pair.IsDeletion())
		})
	}
}

func TestAlignedPair_IsAddition(t *testing.T) {
	additionLine := DiffLine{Type: LineAddition, Content: "added"}
	contextLine := DiffLine{Type: LineContext, Content: "context"}

	tests := []struct {
		name     string
		pair     alignedPair
		expected bool
	}{
		{
			name:     "addition with nil left",
			pair:     alignedPair{Left: nil, Right: &additionLine},
			expected: true,
		},
		{
			name:     "addition with non-nil left",
			pair:     alignedPair{Left: &contextLine, Right: &additionLine},
			expected: false,
		},
		{
			name:     "context with nil left",
			pair:     alignedPair{Left: nil, Right: &contextLine},
			expected: false,
		},
		{
			name:     "nil right",
			pair:     alignedPair{Left: &additionLine, Right: nil},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pair.IsAddition())
		})
	}
}

func TestAlignedPair_IsModification(t *testing.T) {
	deletionLine := DiffLine{Type: LineDeletion, Content: "old"}
	additionLine := DiffLine{Type: LineAddition, Content: "new"}
	contextLine := DiffLine{Type: LineContext, Content: "context"}

	tests := []struct {
		name     string
		pair     alignedPair
		expected bool
	}{
		{
			name:     "deletion left, addition right",
			pair:     alignedPair{Left: &deletionLine, Right: &additionLine},
			expected: true,
		},
		{
			name:     "both context",
			pair:     alignedPair{Left: &contextLine, Right: &contextLine},
			expected: false,
		},
		{
			name:     "left nil",
			pair:     alignedPair{Left: nil, Right: &additionLine},
			expected: false,
		},
		{
			name:     "right nil",
			pair:     alignedPair{Left: &deletionLine, Right: nil},
			expected: false,
		},
		{
			name:     "both deletions",
			pair:     alignedPair{Left: &deletionLine, Right: &deletionLine},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pair.IsModification())
		})
	}
}

func TestAlignedPair_IsHunkHeader(t *testing.T) {
	headerLine := DiffLine{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@"}
	contextLine := DiffLine{Type: LineContext, Content: "context"}

	tests := []struct {
		name     string
		pair     alignedPair
		expected bool
	}{
		{
			name:     "hunk header on left",
			pair:     alignedPair{Left: &headerLine, Right: nil},
			expected: true,
		},
		{
			name:     "hunk header with right (unusual)",
			pair:     alignedPair{Left: &headerLine, Right: &contextLine},
			expected: true,
		},
		{
			name:     "context line",
			pair:     alignedPair{Left: &contextLine, Right: nil},
			expected: false,
		},
		{
			name:     "nil left",
			pair:     alignedPair{Left: nil, Right: &headerLine},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.pair.IsHunkHeader())
		})
	}
}

// Tests for alignHunk

func TestAlignHunk_EmptyHunk(t *testing.T) {
	hunk := DiffHunk{Lines: []DiffLine{}}
	pairs := alignHunk(hunk)
	require.Nil(t, pairs, "empty hunk should return nil")
}

func TestAlignHunk_ContextOnly(t *testing.T) {
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "line 1"},
			{Type: LineContext, OldLineNum: 2, NewLineNum: 2, Content: "line 2"},
			{Type: LineContext, OldLineNum: 3, NewLineNum: 3, Content: "line 3"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 3)

	for i, pair := range pairs {
		require.True(t, pair.IsContext(), "pair %d should be context", i)
		require.NotNil(t, pair.Left)
		require.NotNil(t, pair.Right)
		// For context, Left and Right point to same line
		require.Equal(t, pair.Left, pair.Right)
	}
}

func TestAlignHunk_PureAdditions(t *testing.T) {
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineAddition, NewLineNum: 1, Content: "new line 1"},
			{Type: LineAddition, NewLineNum: 2, Content: "new line 2"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 2)

	for i, pair := range pairs {
		require.True(t, pair.IsAddition(), "pair %d should be addition", i)
		require.Nil(t, pair.Left)
		require.NotNil(t, pair.Right)
		require.Equal(t, LineAddition, pair.Right.Type)
	}
}

func TestAlignHunk_PureDeletions(t *testing.T) {
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineDeletion, OldLineNum: 1, Content: "old line 1"},
			{Type: LineDeletion, OldLineNum: 2, Content: "old line 2"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 2)

	for i, pair := range pairs {
		require.True(t, pair.IsDeletion(), "pair %d should be deletion", i)
		require.NotNil(t, pair.Left)
		require.Nil(t, pair.Right)
		require.Equal(t, LineDeletion, pair.Left.Type)
	}
}

func TestAlignHunk_Modifications(t *testing.T) {
	// Adjacent delete+add pairs should be treated as modifications
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineDeletion, OldLineNum: 1, Content: "old content"},
			{Type: LineAddition, NewLineNum: 1, Content: "new content"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 1)

	pair := pairs[0]
	require.True(t, pair.IsModification())
	require.Equal(t, "old content", pair.Left.Content)
	require.Equal(t, "new content", pair.Right.Content)
}

func TestAlignHunk_MultipleModifications(t *testing.T) {
	// Multiple adjacent delete+add should pair 1:1
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineDeletion, OldLineNum: 1, Content: "old 1"},
			{Type: LineDeletion, OldLineNum: 2, Content: "old 2"},
			{Type: LineAddition, NewLineNum: 1, Content: "new 1"},
			{Type: LineAddition, NewLineNum: 2, Content: "new 2"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 2)

	require.True(t, pairs[0].IsModification())
	require.Equal(t, "old 1", pairs[0].Left.Content)
	require.Equal(t, "new 1", pairs[0].Right.Content)

	require.True(t, pairs[1].IsModification())
	require.Equal(t, "old 2", pairs[1].Left.Content)
	require.Equal(t, "new 2", pairs[1].Right.Content)
}

func TestAlignHunk_MoreDeletionsThanAdditions(t *testing.T) {
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineDeletion, OldLineNum: 1, Content: "del 1"},
			{Type: LineDeletion, OldLineNum: 2, Content: "del 2"},
			{Type: LineDeletion, OldLineNum: 3, Content: "del 3"},
			{Type: LineAddition, NewLineNum: 1, Content: "add 1"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 3)

	// First pair is modification
	require.True(t, pairs[0].IsModification())
	require.Equal(t, "del 1", pairs[0].Left.Content)
	require.Equal(t, "add 1", pairs[0].Right.Content)

	// Remaining are pure deletions
	require.True(t, pairs[1].IsDeletion())
	require.Equal(t, "del 2", pairs[1].Left.Content)

	require.True(t, pairs[2].IsDeletion())
	require.Equal(t, "del 3", pairs[2].Left.Content)
}

func TestAlignHunk_MoreAdditionsThanDeletions(t *testing.T) {
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineDeletion, OldLineNum: 1, Content: "del 1"},
			{Type: LineAddition, NewLineNum: 1, Content: "add 1"},
			{Type: LineAddition, NewLineNum: 2, Content: "add 2"},
			{Type: LineAddition, NewLineNum: 3, Content: "add 3"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 3)

	// First pair is modification
	require.True(t, pairs[0].IsModification())
	require.Equal(t, "del 1", pairs[0].Left.Content)
	require.Equal(t, "add 1", pairs[0].Right.Content)

	// Remaining are pure additions
	require.True(t, pairs[1].IsAddition())
	require.Equal(t, "add 2", pairs[1].Right.Content)

	require.True(t, pairs[2].IsAddition())
	require.Equal(t, "add 3", pairs[2].Right.Content)
}

func TestAlignHunk_HunkHeader(t *testing.T) {
	hunk := DiffHunk{
		Header: "@@ -1,3 +1,4 @@ func main()",
		Lines: []DiffLine{
			{Type: LineHunkHeader, Content: "@@ -1,3 +1,4 @@ func main()"},
			{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "context"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 2)

	// Hunk header
	require.True(t, pairs[0].IsHunkHeader())
	require.NotNil(t, pairs[0].Left)
	require.Nil(t, pairs[0].Right)

	// Context
	require.True(t, pairs[1].IsContext())
}

func TestAlignHunk_MixedChanges(t *testing.T) {
	// Realistic scenario: context, modification, pure addition, more context
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineHunkHeader, Content: "@@ -1,5 +1,6 @@"},
			{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "package main"},
			{Type: LineContext, OldLineNum: 2, NewLineNum: 2, Content: ""},
			{Type: LineDeletion, OldLineNum: 3, Content: "func old() {}"},
			{Type: LineAddition, NewLineNum: 3, Content: "func new() {}"},
			{Type: LineAddition, NewLineNum: 4, Content: "func extra() {}"},
			{Type: LineContext, OldLineNum: 4, NewLineNum: 5, Content: ""},
			{Type: LineContext, OldLineNum: 5, NewLineNum: 6, Content: "func main() {}"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 7)

	// Header
	require.True(t, pairs[0].IsHunkHeader())

	// Context lines
	require.True(t, pairs[1].IsContext())
	require.True(t, pairs[2].IsContext())

	// Modification
	require.True(t, pairs[3].IsModification())
	require.Equal(t, "func old() {}", pairs[3].Left.Content)
	require.Equal(t, "func new() {}", pairs[3].Right.Content)

	// Pure addition
	require.True(t, pairs[4].IsAddition())
	require.Equal(t, "func extra() {}", pairs[4].Right.Content)

	// More context
	require.True(t, pairs[5].IsContext())
	require.True(t, pairs[6].IsContext())
}

func TestAlignHunk_AdditionsThenDeletions(t *testing.T) {
	// Additions followed by deletions (not adjacent delete+add)
	// Should NOT be paired as modifications
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineAddition, NewLineNum: 1, Content: "added first"},
			{Type: LineDeletion, OldLineNum: 1, Content: "deleted after"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 2)

	// Pure addition (comes first)
	require.True(t, pairs[0].IsAddition())
	require.Equal(t, "added first", pairs[0].Right.Content)

	// Pure deletion (comes after)
	require.True(t, pairs[1].IsDeletion())
	require.Equal(t, "deleted after", pairs[1].Left.Content)
}

func TestAlignHunk_InterleaveContextWithChanges(t *testing.T) {
	// Context, delete, context, add - should NOT pair as modification
	hunk := DiffHunk{
		Lines: []DiffLine{
			{Type: LineContext, OldLineNum: 1, NewLineNum: 1, Content: "ctx 1"},
			{Type: LineDeletion, OldLineNum: 2, Content: "deleted"},
			{Type: LineContext, OldLineNum: 3, NewLineNum: 2, Content: "ctx 2"},
			{Type: LineAddition, NewLineNum: 3, Content: "added"},
		},
	}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 4)

	require.True(t, pairs[0].IsContext())
	require.True(t, pairs[1].IsDeletion())
	require.True(t, pairs[2].IsContext())
	require.True(t, pairs[3].IsAddition())
}

func TestAlignHunk_PreservesLineReferences(t *testing.T) {
	// Verify that pointers actually point to the original lines
	lines := []DiffLine{
		{Type: LineDeletion, OldLineNum: 10, Content: "delete me"},
		{Type: LineAddition, NewLineNum: 10, Content: "add me"},
	}
	hunk := DiffHunk{Lines: lines}

	pairs := alignHunk(hunk)
	require.Len(t, pairs, 1)

	// Check that the pointers reference the original slice elements
	require.Equal(t, 10, pairs[0].Left.OldLineNum)
	require.Equal(t, 10, pairs[0].Right.NewLineNum)
	require.Equal(t, "delete me", pairs[0].Left.Content)
	require.Equal(t, "add me", pairs[0].Right.Content)
}
