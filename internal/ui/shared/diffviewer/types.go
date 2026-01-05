// Package diffviewer provides a TUI component for viewing git diffs.
package diffviewer

// ViewMode represents the diff display mode.
type ViewMode int

const (
	// ViewModeUnified shows changes in a single column with +/- markers.
	ViewModeUnified ViewMode = iota
	// ViewModeSideBySide shows old and new versions in parallel columns.
	ViewModeSideBySide
)

// String returns a human-readable name for the view mode.
func (m ViewMode) String() string {
	switch m {
	case ViewModeUnified:
		return "UNIFIED"
	case ViewModeSideBySide:
		return "SIDE-BY-SIDE"
	default:
		return "UNKNOWN"
	}
}

// ErrorCategory represents the category of a diff error for recovery actions.
type ErrorCategory int

const (
	// ErrCategoryParse indicates a parse error - show raw diff option.
	ErrCategoryParse ErrorCategory = iota
	// ErrCategoryGitOp indicates a git operation error - retry button.
	ErrCategoryGitOp
	// ErrCategoryPermission indicates a permission error - explain & suggest.
	ErrCategoryPermission
	// ErrCategoryConflict indicates a conflict error - reload button.
	ErrCategoryConflict
	// ErrCategoryTimeout indicates a timeout error - retry with longer timeout.
	ErrCategoryTimeout
)

// String returns a human-readable name for the error category.
func (c ErrorCategory) String() string {
	switch c {
	case ErrCategoryParse:
		return "Parse Error"
	case ErrCategoryGitOp:
		return "Git Operation Error"
	case ErrCategoryPermission:
		return "Permission Error"
	case ErrCategoryConflict:
		return "Conflict Error"
	case ErrCategoryTimeout:
		return "Timeout Error"
	default:
		return "Unknown Error"
	}
}

// DiffError represents an error with recovery information for the diffviewer.
type DiffError struct {
	Category ErrorCategory // Type of error for determining recovery actions
	Message  string        // Human-readable error message
	HelpText string        // Additional guidance for the user
}

// Error implements the error interface.
func (e DiffError) Error() string {
	return e.Message
}

// NewDiffError creates a new DiffError with the given category and message.
func NewDiffError(category ErrorCategory, message string) DiffError {
	return DiffError{
		Category: category,
		Message:  message,
	}
}

// WithHelpText sets additional help text for the error.
func (e DiffError) WithHelpText(help string) DiffError {
	e.HelpText = help
	return e
}

// LineType represents the type of a diff line.
type LineType int

const (
	LineContext    LineType = iota // ' ' prefix - unchanged line
	LineAddition                   // '+' prefix - added line
	LineDeletion                   // '-' prefix - deleted line
	LineHunkHeader                 // '@@ ... @@' - hunk marker
)

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	Type       LineType // Addition, Deletion, Context, or HunkHeader
	OldLineNum int      // Line number in old file (0 if addition)
	NewLineNum int      // Line number in new file (0 if deletion)
	Content    string   // Line content without +/- prefix
}

// DiffHunk represents a contiguous section of changes in a diff.
type DiffHunk struct {
	OldStart int    // Starting line number in old file
	OldCount int    // Number of lines from old file
	NewStart int    // Starting line number in new file
	NewCount int    // Number of lines from new file
	Header   string // The @@ line text
	Lines    []DiffLine
}

// DiffFile represents a single file's changes in a diff.
type DiffFile struct {
	OldPath     string // Path in old version (or /dev/null for new files)
	NewPath     string // Path in new version (or /dev/null for deleted files)
	Additions   int    // Count of added lines
	Deletions   int    // Count of deleted lines
	IsBinary    bool   // True if file is binary
	IsRenamed   bool   // True if file was renamed
	IsNew       bool   // True if new file (OldPath = /dev/null)
	IsDeleted   bool   // True if deleted file (NewPath = /dev/null)
	IsUntracked bool   // True if untracked file (not yet staged)
	Similarity  int    // Rename similarity percentage (0-100)
	Hunks       []DiffHunk
}

// alignedPair represents one row in side-by-side diff view.
// It pairs corresponding lines from old and new versions of a file.
//
// Alignment rules (GapAlignment strategy):
//   - Deletions: Left has content, Right is nil (blank row on right)
//   - Additions: Left is nil, Right has content (blank row on left)
//   - Context: Both Left and Right have the same content
//   - Modifications: Adjacent delete+add pairs are paired as Left/Right
type alignedPair struct {
	Left  *DiffLine // Line from old file (nil for insertion-only row)
	Right *DiffLine // Line from new file (nil for deletion-only row)
}

// IsContext returns true if both sides have context (unchanged) content.
func (p alignedPair) IsContext() bool {
	return p.Left != nil && p.Right != nil &&
		p.Left.Type == LineContext && p.Right.Type == LineContext
}

// IsDeletion returns true if only the left side has content (line was deleted).
func (p alignedPair) IsDeletion() bool {
	return p.Left != nil && p.Right == nil && p.Left.Type == LineDeletion
}

// IsAddition returns true if only the right side has content (line was added).
func (p alignedPair) IsAddition() bool {
	return p.Left == nil && p.Right != nil && p.Right.Type == LineAddition
}

// IsModification returns true if both sides have content with different types
// (typically a deletion paired with an addition representing a modified line).
func (p alignedPair) IsModification() bool {
	return p.Left != nil && p.Right != nil &&
		p.Left.Type == LineDeletion && p.Right.Type == LineAddition
}

// IsHunkHeader returns true if this pair represents a hunk header.
// Hunk headers appear on the left side only for display purposes.
func (p alignedPair) IsHunkHeader() bool {
	return p.Left != nil && p.Left.Type == LineHunkHeader
}

// alignHunk converts a hunk's lines into aligned pairs for side-by-side display.
// It implements the GapAlignment strategy:
//   - Context lines appear on both sides
//   - Deletions appear on left with blank right
//   - Additions appear on right with blank left
//   - Adjacent delete+add sequences are paired as modifications
//
// The algorithm processes lines in order, collecting consecutive deletions
// and additions, then pairing them together. When deletions and additions
// are adjacent, they are paired as modifications (representing changed lines).
func alignHunk(hunk DiffHunk) []alignedPair {
	if len(hunk.Lines) == 0 {
		return nil
	}

	var pairs []alignedPair
	lines := hunk.Lines
	i := 0

	for i < len(lines) {
		line := lines[i]

		switch line.Type {
		case LineHunkHeader:
			// Hunk headers go on left side only
			pairs = append(pairs, alignedPair{
				Left:  &lines[i],
				Right: nil,
			})
			i++

		case LineContext:
			// Context lines appear on both sides
			pairs = append(pairs, alignedPair{
				Left:  &lines[i],
				Right: &lines[i],
			})
			i++

		case LineDeletion:
			// Collect consecutive deletions
			deletions := collectConsecutive(lines, i, LineDeletion)

			// Check for following additions to pair with
			nextIdx := i + len(deletions)
			var additions []int
			if nextIdx < len(lines) {
				additions = collectConsecutive(lines, nextIdx, LineAddition)
			}

			// Pair deletions with additions (modifications)
			// Then emit remaining deletions or additions as unpaired
			pairs = append(pairs, pairDeletionsAndAdditions(lines, deletions, additions)...)

			i = nextIdx + len(additions)

		case LineAddition:
			// Pure additions (not following deletions)
			// These get blank left side
			pairs = append(pairs, alignedPair{
				Left:  nil,
				Right: &lines[i],
			})
			i++
		}
	}

	return pairs
}

// collectConsecutive returns indices of consecutive lines of the given type
// starting at startIdx.
func collectConsecutive(lines []DiffLine, startIdx int, lineType LineType) []int {
	var indices []int
	for i := startIdx; i < len(lines) && lines[i].Type == lineType; i++ {
		indices = append(indices, i)
	}
	return indices
}

// pairDeletionsAndAdditions creates aligned pairs from deletion and addition indices.
// It pairs them 1:1 as modifications, then emits extras as unpaired.
func pairDeletionsAndAdditions(lines []DiffLine, deletions, additions []int) []alignedPair {
	var pairs []alignedPair

	// Pair deletions with additions (modifications)
	minLen := min(len(deletions), len(additions))
	for j := range minLen {
		pairs = append(pairs, alignedPair{
			Left:  &lines[deletions[j]],
			Right: &lines[additions[j]],
		})
	}

	// Remaining unpaired deletions
	for j := minLen; j < len(deletions); j++ {
		pairs = append(pairs, alignedPair{
			Left:  &lines[deletions[j]],
			Right: nil,
		})
	}

	// Remaining unpaired additions
	for j := minLen; j < len(additions); j++ {
		pairs = append(pairs, alignedPair{
			Left:  nil,
			Right: &lines[additions[j]],
		})
	}

	return pairs
}
