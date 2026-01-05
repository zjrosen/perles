package diffviewer

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Word diff constants for performance bounds.
const (
	// WordDiffMaxLineLength skips word diff for lines exceeding this length.
	WordDiffMaxLineLength = 500
	// WordDiffMaxPairs limits word diff computation to first N pairs per hunk.
	WordDiffMaxPairs = 100
	// WordDiffTimeout is the maximum time allowed for word diff per file.
	WordDiffTimeout = 50 * time.Millisecond
)

// wordSegmentType indicates whether a segment is unchanged, added, or deleted.
type wordSegmentType int

const (
	// segmentUnchanged represents unchanged text.
	segmentUnchanged wordSegmentType = iota
	// segmentAdded represents added text.
	segmentAdded
	// segmentDeleted represents deleted text.
	segmentDeleted
)

// wordSegment represents a segment of text with its diff status.
type wordSegment struct {
	Type wordSegmentType
	Text string
}

// wordDiffResult contains the word-level diff results for a line pair.
type wordDiffResult struct {
	OldSegments []wordSegment // Segments for the deleted line
	NewSegments []wordSegment // Segments for the added line
}

// linePair represents an adjacent deletion+addition pair for word diffing.
type linePair struct {
	DeletedLine DiffLine
	AddedLine   DiffLine
	DeletedIdx  int // Index in hunk.Lines
	AddedIdx    int // Index in hunk.Lines
}

// tokenize splits a line into tokens (words and punctuation).
// Example: "foo.bar.baz()" → ["foo", ".", "bar", ".", "baz", "(", ")"]
func tokenize(line string) []string {
	if line == "" {
		return nil
	}

	var tokens []string
	var current strings.Builder

	for _, r := range line {
		if unicode.IsSpace(r) {
			// Flush current token
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Add whitespace as its own token
			tokens = append(tokens, string(r))
		} else if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			// Flush current token
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// Add punctuation as its own token
			tokens = append(tokens, string(r))
		} else {
			// Alphanumeric - accumulate into word
			current.WriteRune(r)
		}
	}

	// Flush final token
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// computeWordDiff computes word-level diff between two lines.
// Returns segments for both the old (deleted) and new (added) line.
func computeWordDiff(oldLine, newLine string) wordDiffResult {
	// Handle edge cases
	if oldLine == "" && newLine == "" {
		return wordDiffResult{}
	}
	if oldLine == "" {
		return wordDiffResult{
			NewSegments: []wordSegment{{Type: segmentAdded, Text: newLine}},
		}
	}
	if newLine == "" {
		return wordDiffResult{
			OldSegments: []wordSegment{{Type: segmentDeleted, Text: oldLine}},
		}
	}

	// Tokenize both lines
	oldTokens := tokenize(oldLine)
	newTokens := tokenize(newLine)

	// Use go-diff to compute diff at token level
	dmp := diffmatchpatch.New()

	// Convert tokens to strings for diff
	oldText := strings.Join(oldTokens, "\x00")
	newText := strings.Join(newTokens, "\x00")

	diffs := dmp.DiffMain(oldText, newText, false)
	diffs = dmp.DiffCleanupSemantic(diffs)

	// Convert diffs back to segments
	var oldSegments, newSegments []wordSegment

	for _, d := range diffs {
		// Split by our token delimiter to get individual tokens
		text := strings.ReplaceAll(d.Text, "\x00", "")

		if text == "" {
			continue
		}

		switch d.Type {
		case diffmatchpatch.DiffEqual:
			oldSegments = append(oldSegments, wordSegment{
				Type: segmentUnchanged,
				Text: text,
			})
			newSegments = append(newSegments, wordSegment{
				Type: segmentUnchanged,
				Text: text,
			})
		case diffmatchpatch.DiffDelete:
			oldSegments = append(oldSegments, wordSegment{
				Type: segmentDeleted,
				Text: text,
			})
		case diffmatchpatch.DiffInsert:
			newSegments = append(newSegments, wordSegment{
				Type: segmentAdded,
				Text: text,
			})
		}
	}

	return wordDiffResult{
		OldSegments: oldSegments,
		NewSegments: newSegments,
	}
}

// findLinePairs finds adjacent delete+add line pairs in a hunk.
// These pairs are candidates for word-level diff highlighting.
func findLinePairs(hunk DiffHunk) []linePair {
	var pairs []linePair

	for i := 0; i < len(hunk.Lines)-1; i++ {
		// Look for deletion followed by addition
		if hunk.Lines[i].Type == LineDeletion && hunk.Lines[i+1].Type == LineAddition {
			pairs = append(pairs, linePair{
				DeletedLine: hunk.Lines[i],
				AddedLine:   hunk.Lines[i+1],
				DeletedIdx:  i,
				AddedIdx:    i + 1,
			})
			// Skip the addition since we've paired it
			i++
		}
	}

	return pairs
}

// hunkWordDiff contains word diff results for all eligible pairs in a hunk.
type hunkWordDiff struct {
	// Results maps line index to word diff result.
	// For a deletion line index, check OldSegments.
	// For an addition line index, check NewSegments.
	Results map[int]wordDiffResult
}

// computeHunkWordDiff computes word-level diffs for a hunk.
// Respects performance bounds: max line length, max pairs, and timeout.
func computeHunkWordDiff(ctx context.Context, hunk DiffHunk) hunkWordDiff {
	result := hunkWordDiff{
		Results: make(map[int]wordDiffResult),
	}

	pairs := findLinePairs(hunk)
	if len(pairs) == 0 {
		return result
	}

	// Limit pairs per hunk
	if len(pairs) > WordDiffMaxPairs {
		pairs = pairs[:WordDiffMaxPairs]
	}

	for _, pair := range pairs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return result
		default:
		}

		// Skip lines that are too long
		if len(pair.DeletedLine.Content) > WordDiffMaxLineLength ||
			len(pair.AddedLine.Content) > WordDiffMaxLineLength {
			continue
		}

		wordDiff := computeWordDiff(pair.DeletedLine.Content, pair.AddedLine.Content)
		result.Results[pair.DeletedIdx] = wordDiff
		result.Results[pair.AddedIdx] = wordDiff
	}

	return result
}

// fileWordDiff contains word diff results for all hunks in a file.
type fileWordDiff struct {
	// HunkDiffs maps hunk index to hunk word diff results.
	HunkDiffs map[int]hunkWordDiff
	// TimedOut indicates if computation was stopped due to timeout.
	TimedOut bool
}

// computeFileWordDiff computes word-level diffs for an entire file.
// Enforces the 50ms timeout per file.
func computeFileWordDiff(file DiffFile) fileWordDiff {
	result := fileWordDiff{
		HunkDiffs: make(map[int]hunkWordDiff),
	}

	if len(file.Hunks) == 0 {
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), WordDiffTimeout)
	defer cancel()

	for i, hunk := range file.Hunks {
		// Check timeout
		select {
		case <-ctx.Done():
			result.TimedOut = true
			return result
		default:
		}

		hunkDiff := computeHunkWordDiff(ctx, hunk)
		if len(hunkDiff.Results) > 0 {
			result.HunkDiffs[i] = hunkDiff
		}
	}

	return result
}

// getSegmentsForLine returns word segments for a specific line if word diff was computed.
// Returns nil if no word diff is available for this line.
func (f fileWordDiff) getSegmentsForLine(hunkIdx, lineIdx int, lineType LineType) []wordSegment {
	hunkDiff, ok := f.HunkDiffs[hunkIdx]
	if !ok {
		return nil
	}

	wordDiff, ok := hunkDiff.Results[lineIdx]
	if !ok {
		return nil
	}

	switch lineType {
	case LineDeletion:
		return wordDiff.OldSegments
	case LineAddition:
		return wordDiff.NewSegments
	default:
		return nil
	}
}
