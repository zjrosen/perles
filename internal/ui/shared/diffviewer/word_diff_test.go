package diffviewer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "simple word",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "two words",
			input:    "hello world",
			expected: []string{"hello", " ", "world"},
		},
		{
			name:     "dotted identifier",
			input:    "foo.bar.baz()",
			expected: []string{"foo", ".", "bar", ".", "baz", "(", ")"},
		},
		{
			name:     "function call",
			input:    "fmt.Printf(\"hello\")",
			expected: []string{"fmt", ".", "Printf", "(", "\"", "hello", "\"", ")"},
		},
		{
			name:     "assignment with spaces",
			input:    "x := foo()",
			expected: []string{"x", " ", ":", "=", " ", "foo", "(", ")"},
		},
		{
			name:     "multiple spaces",
			input:    "a    b",
			expected: []string{"a", " ", " ", " ", " ", "b"},
		},
		{
			name:     "tabs",
			input:    "a\tb",
			expected: []string{"a", "\t", "b"},
		},
		{
			name:     "numbers",
			input:    "count = 42",
			expected: []string{"count", " ", "=", " ", "42"},
		},
		{
			name:     "brackets and braces",
			input:    "arr[0] = map[string]int{}",
			expected: []string{"arr", "[", "0", "]", " ", "=", " ", "map", "[", "string", "]", "int", "{", "}"},
		},
		{
			name:     "operators",
			input:    "a + b - c * d / e",
			expected: []string{"a", " ", "+", " ", "b", " ", "-", " ", "c", " ", "*", " ", "d", " ", "/", " ", "e"},
		},
		{
			name:     "leading whitespace",
			input:    "  indented",
			expected: []string{" ", " ", "indented"},
		},
		{
			name:     "trailing whitespace",
			input:    "text  ",
			expected: []string{"text", " ", " "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestComputeWordDiff(t *testing.T) {
	tests := []struct {
		name           string
		oldLine        string
		newLine        string
		expectOldTypes []wordSegmentType
		expectNewTypes []wordSegmentType
		expectOldTexts []string
		expectNewTexts []string
	}{
		{
			name:    "both empty",
			oldLine: "",
			newLine: "",
		},
		{
			name:           "old empty",
			oldLine:        "",
			newLine:        "hello",
			expectNewTypes: []wordSegmentType{segmentAdded},
			expectNewTexts: []string{"hello"},
		},
		{
			name:           "new empty",
			oldLine:        "hello",
			newLine:        "",
			expectOldTypes: []wordSegmentType{segmentDeleted},
			expectOldTexts: []string{"hello"},
		},
		{
			name:           "identical lines",
			oldLine:        "hello world",
			newLine:        "hello world",
			expectOldTypes: []wordSegmentType{segmentUnchanged},
			expectNewTypes: []wordSegmentType{segmentUnchanged},
			expectOldTexts: []string{"hello world"},
			expectNewTexts: []string{"hello world"},
		},
		{
			name:           "single word change",
			oldLine:        "hello world",
			newLine:        "hello universe",
			expectOldTypes: []wordSegmentType{segmentUnchanged, segmentDeleted},
			expectNewTypes: []wordSegmentType{segmentUnchanged, segmentAdded},
			expectOldTexts: []string{"hello ", "world"},
			expectNewTexts: []string{"hello ", "universe"},
		},
		{
			name:    "added word",
			oldLine: "foo bar",
			newLine: "foo baz bar",
			// The diff algorithm may produce different segmentation
			// We just verify the segments exist and contain expected content
		},
		{
			name:    "function rename",
			oldLine: "fmt.Println(x)",
			newLine: "log.Printf(x)",
			// Complex diff - just verify segments are produced
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeWordDiff(tt.oldLine, tt.newLine)

			// Check old segments
			if tt.expectOldTypes != nil {
				require.Len(t, result.OldSegments, len(tt.expectOldTypes), "old segments count mismatch")
				for i, seg := range result.OldSegments {
					require.Equal(t, tt.expectOldTypes[i], seg.Type, "old segment %d type mismatch", i)
					if tt.expectOldTexts != nil && i < len(tt.expectOldTexts) {
						require.Equal(t, tt.expectOldTexts[i], seg.Text, "old segment %d text mismatch", i)
					}
				}
			}

			// Check new segments
			if tt.expectNewTypes != nil {
				require.Len(t, result.NewSegments, len(tt.expectNewTypes), "new segments count mismatch")
				for i, seg := range result.NewSegments {
					require.Equal(t, tt.expectNewTypes[i], seg.Type, "new segment %d type mismatch", i)
					if tt.expectNewTexts != nil && i < len(tt.expectNewTexts) {
						require.Equal(t, tt.expectNewTexts[i], seg.Text, "new segment %d text mismatch", i)
					}
				}
			}

			// For tests without specific expectations, verify basic properties
			if tt.expectOldTypes == nil && tt.expectNewTypes == nil {
				// Reconstruct the original lines from segments
				if tt.oldLine != "" {
					var oldText strings.Builder
					for _, seg := range result.OldSegments {
						oldText.WriteString(seg.Text)
					}
					require.Equal(t, tt.oldLine, oldText.String(), "old line reconstruction mismatch")
				}
				if tt.newLine != "" {
					var newText strings.Builder
					for _, seg := range result.NewSegments {
						newText.WriteString(seg.Text)
					}
					require.Equal(t, tt.newLine, newText.String(), "new line reconstruction mismatch")
				}
			}
		})
	}
}

func TestFindLinePairs(t *testing.T) {
	tests := []struct {
		name          string
		lines         []DiffLine
		expectedPairs int
	}{
		{
			name:          "empty hunk",
			lines:         []DiffLine{},
			expectedPairs: 0,
		},
		{
			name: "no pairs - only deletions",
			lines: []DiffLine{
				{Type: LineDeletion, Content: "old1"},
				{Type: LineDeletion, Content: "old2"},
			},
			expectedPairs: 0,
		},
		{
			name: "no pairs - only additions",
			lines: []DiffLine{
				{Type: LineAddition, Content: "new1"},
				{Type: LineAddition, Content: "new2"},
			},
			expectedPairs: 0,
		},
		{
			name: "one pair",
			lines: []DiffLine{
				{Type: LineDeletion, Content: "old"},
				{Type: LineAddition, Content: "new"},
			},
			expectedPairs: 1,
		},
		{
			name: "two consecutive pairs",
			lines: []DiffLine{
				{Type: LineDeletion, Content: "old1"},
				{Type: LineAddition, Content: "new1"},
				{Type: LineDeletion, Content: "old2"},
				{Type: LineAddition, Content: "new2"},
			},
			expectedPairs: 2,
		},
		{
			name: "pair with context around",
			lines: []DiffLine{
				{Type: LineContext, Content: "context before"},
				{Type: LineDeletion, Content: "old"},
				{Type: LineAddition, Content: "new"},
				{Type: LineContext, Content: "context after"},
			},
			expectedPairs: 1,
		},
		{
			name: "addition before deletion - no pair",
			lines: []DiffLine{
				{Type: LineAddition, Content: "new"},
				{Type: LineDeletion, Content: "old"},
			},
			expectedPairs: 0,
		},
		{
			name: "del-del-add-add finds middle pair",
			lines: []DiffLine{
				{Type: LineDeletion, Content: "old1"},
				{Type: LineDeletion, Content: "old2"},
				{Type: LineAddition, Content: "new1"},
				{Type: LineAddition, Content: "new2"},
			},
			expectedPairs: 1, // old2+new1 forms a pair at indices 1,2
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunk := DiffHunk{Lines: tt.lines}
			pairs := findLinePairs(hunk)
			require.Len(t, pairs, tt.expectedPairs)
		})
	}
}

func TestComputeHunkWordDiff(t *testing.T) {
	t.Run("basic pair", func(t *testing.T) {
		hunk := DiffHunk{
			Lines: []DiffLine{
				{Type: LineDeletion, Content: "old line"},
				{Type: LineAddition, Content: "new line"},
			},
		}

		ctx := context.Background()
		result := computeHunkWordDiff(ctx, hunk)

		require.Len(t, result.Results, 2)
		require.Contains(t, result.Results, 0) // deletion index
		require.Contains(t, result.Results, 1) // addition index
	})

	t.Run("skips long lines", func(t *testing.T) {
		longLine := strings.Repeat("x", WordDiffMaxLineLength+1)
		hunk := DiffHunk{
			Lines: []DiffLine{
				{Type: LineDeletion, Content: longLine},
				{Type: LineAddition, Content: "short"},
			},
		}

		ctx := context.Background()
		result := computeHunkWordDiff(ctx, hunk)

		require.Empty(t, result.Results, "should skip pairs with long lines")
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		// Create a hunk with many pairs
		lines := make([]DiffLine, 200)
		for i := 0; i < 200; i += 2 {
			lines[i] = DiffLine{Type: LineDeletion, Content: "old"}
			lines[i+1] = DiffLine{Type: LineAddition, Content: "new"}
		}
		hunk := DiffHunk{Lines: lines}

		// Cancel immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := computeHunkWordDiff(ctx, hunk)

		// Should have stopped early
		require.Less(t, len(result.Results), 200)
	})

	t.Run("limits pairs per hunk", func(t *testing.T) {
		// Create a hunk with more than WordDiffMaxPairs pairs
		numPairs := WordDiffMaxPairs + 50
		lines := make([]DiffLine, numPairs*2)
		for i := 0; i < numPairs*2; i += 2 {
			lines[i] = DiffLine{Type: LineDeletion, Content: "old"}
			lines[i+1] = DiffLine{Type: LineAddition, Content: "new"}
		}
		hunk := DiffHunk{Lines: lines}

		ctx := context.Background()
		result := computeHunkWordDiff(ctx, hunk)

		// Should be limited to WordDiffMaxPairs * 2 entries (each pair creates 2)
		require.LessOrEqual(t, len(result.Results), WordDiffMaxPairs*2)
	})
}

func TestComputeFileWordDiff(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		file := DiffFile{Hunks: nil}
		result := computeFileWordDiff(file)

		require.Empty(t, result.HunkDiffs)
		require.False(t, result.TimedOut)
	})

	t.Run("single hunk", func(t *testing.T) {
		file := DiffFile{
			Hunks: []DiffHunk{
				{
					Lines: []DiffLine{
						{Type: LineDeletion, Content: "old"},
						{Type: LineAddition, Content: "new"},
					},
				},
			},
		}
		result := computeFileWordDiff(file)

		require.Len(t, result.HunkDiffs, 1)
		require.Contains(t, result.HunkDiffs, 0)
		require.False(t, result.TimedOut)
	})

	t.Run("multiple hunks", func(t *testing.T) {
		file := DiffFile{
			Hunks: []DiffHunk{
				{
					Lines: []DiffLine{
						{Type: LineDeletion, Content: "old1"},
						{Type: LineAddition, Content: "new1"},
					},
				},
				{
					Lines: []DiffLine{
						{Type: LineDeletion, Content: "old2"},
						{Type: LineAddition, Content: "new2"},
					},
				},
			},
		}
		result := computeFileWordDiff(file)

		require.Len(t, result.HunkDiffs, 2)
		require.Contains(t, result.HunkDiffs, 0)
		require.Contains(t, result.HunkDiffs, 1)
	})

	t.Run("hunk with no pairs is not included", func(t *testing.T) {
		file := DiffFile{
			Hunks: []DiffHunk{
				{
					Lines: []DiffLine{
						{Type: LineAddition, Content: "new only"},
					},
				},
			},
		}
		result := computeFileWordDiff(file)

		require.Empty(t, result.HunkDiffs)
	})
}

func TestFileWordDiff_GetSegmentsForLine(t *testing.T) {
	// Build a result with known data
	wordDiff := fileWordDiff{
		HunkDiffs: map[int]hunkWordDiff{
			0: {
				Results: map[int]wordDiffResult{
					0: {
						OldSegments: []wordSegment{{Type: segmentDeleted, Text: "old"}},
						NewSegments: []wordSegment{{Type: segmentAdded, Text: "new"}},
					},
					1: {
						OldSegments: []wordSegment{{Type: segmentDeleted, Text: "old"}},
						NewSegments: []wordSegment{{Type: segmentAdded, Text: "new"}},
					},
				},
			},
		},
	}

	t.Run("get deletion segments", func(t *testing.T) {
		segments := wordDiff.getSegmentsForLine(0, 0, LineDeletion)
		require.NotNil(t, segments)
		require.Len(t, segments, 1)
		require.Equal(t, segmentDeleted, segments[0].Type)
	})

	t.Run("get addition segments", func(t *testing.T) {
		segments := wordDiff.getSegmentsForLine(0, 1, LineAddition)
		require.NotNil(t, segments)
		require.Len(t, segments, 1)
		require.Equal(t, segmentAdded, segments[0].Type)
	})

	t.Run("returns nil for context lines", func(t *testing.T) {
		segments := wordDiff.getSegmentsForLine(0, 0, LineContext)
		require.Nil(t, segments)
	})

	t.Run("returns nil for missing hunk", func(t *testing.T) {
		segments := wordDiff.getSegmentsForLine(99, 0, LineDeletion)
		require.Nil(t, segments)
	})

	t.Run("returns nil for missing line", func(t *testing.T) {
		segments := wordDiff.getSegmentsForLine(0, 99, LineDeletion)
		require.Nil(t, segments)
	})
}

// ============================================================================
// Performance Tests
// ============================================================================

func TestWordDiffPerformance_Under50ms(t *testing.T) {
	// Generate a realistic file with many hunks and pairs
	file := generateDiffFileWithPairs(100, 20) // 100 hunks, 20 pairs each

	start := time.Now()
	result := computeFileWordDiff(file)
	elapsed := time.Since(start)

	t.Logf("Word diff for %d hunks with %d pairs each: %v", 100, 20, elapsed)
	t.Logf("Timed out: %v", result.TimedOut)

	// Should complete within 50ms or timeout gracefully
	require.True(t, elapsed < 60*time.Millisecond || result.TimedOut,
		"Word diff should complete in <50ms or timeout, took %v", elapsed)
}

func BenchmarkWordDiff_SinglePair(b *testing.B) {
	oldLine := "func processData(ctx context.Context, data []byte) error {"
	newLine := "func processData(ctx context.Context, input []byte) error {"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeWordDiff(oldLine, newLine)
	}
}

func BenchmarkWordDiff_File100Pairs(b *testing.B) {
	file := generateDiffFileWithPairs(10, 10) // 10 hunks, 10 pairs each

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = computeFileWordDiff(file)
	}
}

func BenchmarkTokenize(b *testing.B) {
	line := "func processData(ctx context.Context, data []byte) error {"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tokenize(line)
	}
}

// generateDiffFileWithPairs creates a synthetic diff file for testing.
func generateDiffFileWithPairs(numHunks, pairsPerHunk int) DiffFile {
	hunks := make([]DiffHunk, numHunks)

	for h := 0; h < numHunks; h++ {
		lines := make([]DiffLine, pairsPerHunk*2)
		for p := 0; p < pairsPerHunk; p++ {
			lines[p*2] = DiffLine{
				Type:    LineDeletion,
				Content: "func oldFunction(ctx context.Context) error {",
			}
			lines[p*2+1] = DiffLine{
				Type:    LineAddition,
				Content: "func newFunction(ctx context.Context) error {",
			}
		}
		hunks[h] = DiffHunk{Lines: lines}
	}

	return DiffFile{Hunks: hunks}
}
