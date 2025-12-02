package styles

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/charmbracelet/lipgloss"
)

// Test colors for border rendering tests
var (
	testColorRed    = lipgloss.Color("#FF0000")
	testColorGreen  = lipgloss.Color("#00FF00")
	testColorBlue   = lipgloss.Color("#0000FF")
	testColorPurple = lipgloss.Color("#800080")
)

func TestRenderWithTitleBorder_Basic(t *testing.T) {
	result := RenderWithTitleBorder("content", "Title", 20, 5, false, testColorGreen, testColorGreen)

	// Should contain border characters
	assert.Contains(t, result, "╭", "missing top-left corner")
	assert.Contains(t, result, "╮", "missing top-right corner")
	assert.Contains(t, result, "╰", "missing bottom-left corner")
	assert.Contains(t, result, "╯", "missing bottom-right corner")

	// Should contain title in first line
	lines := strings.Split(result, "\n")
	require.NotEmpty(t, lines, "no lines in result")
	assert.Contains(t, lines[0], "Title", "title not found in first line")
}

func TestRenderWithTitleBorder_Focused(t *testing.T) {
	unfocused := RenderWithTitleBorder("content", "Title", 20, 5, false, testColorGreen, testColorGreen)
	focused := RenderWithTitleBorder("content", "Title", 20, 5, true, testColorGreen, testColorGreen)

	// Both should have same structure but different styling
	unfocusedLines := strings.Split(unfocused, "\n")
	focusedLines := strings.Split(focused, "\n")

	assert.Equal(t, len(unfocusedLines), len(focusedLines), "different line counts")

	// Both should contain title
	assert.Contains(t, unfocused, "Title", "unfocused missing title")
	assert.Contains(t, focused, "Title", "focused missing title")
}

func TestRenderWithTitleBorder_LongTitle(t *testing.T) {
	longTitle := "This Is A Very Long Title That Should Be Truncated"
	result := RenderWithTitleBorder("content", longTitle, 20, 5, false, testColorRed, testColorRed)

	// Should still have valid border structure
	assert.Contains(t, result, "╭", "missing top-left corner")

	lines := strings.Split(result, "\n")
	require.NotEmpty(t, lines, "no lines in result")

	// First line should not exceed width
	firstLineWidth := lipgloss.Width(lines[0])
	assert.LessOrEqual(t, firstLineWidth, 20, "first line too wide: %d > 20", firstLineWidth)

	// Should have truncation indicator
	assert.Contains(t, lines[0], "...", "long title should be truncated with ellipsis")
}

func TestRenderWithTitleBorder_EmptyContent(t *testing.T) {
	result := RenderWithTitleBorder("", "Title", 20, 5, false, testColorBlue, testColorBlue)

	// Should still render proper border
	assert.Contains(t, result, "╭", "missing top-left corner")
	assert.Contains(t, result, "Title", "missing title")

	// Should have correct number of lines
	lines := strings.Split(result, "\n")
	// 1 top border + 3 content lines (height 5 - 2 borders) + 1 bottom border = 5
	assert.Len(t, lines, 5, "expected 5 lines")
}

func TestRenderWithTitleBorder_NarrowWidth(t *testing.T) {
	result := RenderWithTitleBorder("x", "T", 6, 3, false, testColorPurple, testColorPurple)

	// Should still render something valid
	assert.Contains(t, result, "╭", "missing top-left corner")
	assert.Contains(t, result, "╯", "missing bottom-right corner")

	// Check line widths
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		assert.LessOrEqual(t, w, 6, "line %d too wide: %d > 6, content: %q", i, w, line)
	}
}

func TestRenderWithTitleBorder_MinimalWidth(t *testing.T) {
	result := RenderWithTitleBorder("", "", 3, 3, false, BorderDefaultColor, BorderDefaultColor)

	// Should handle minimal size gracefully
	assert.Contains(t, result, "╭", "missing top-left corner")
	assert.Contains(t, result, "╯", "missing bottom-right corner")
}

func TestRenderWithTitleBorder_EmptyTitle(t *testing.T) {
	result := RenderWithTitleBorder("content", "", 20, 5, false, testColorGreen, testColorGreen)

	// First line should just be a plain border
	lines := strings.Split(result, "\n")
	require.NotEmpty(t, lines, "no lines in result")

	// Should start with top-left and be all dashes (no title text)
	assert.True(t, strings.HasPrefix(lines[0], "╭"), "should start with top-left corner")
}

func TestRenderWithTitleBorder_MultilineContent(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3"
	result := RenderWithTitleBorder(content, "Title", 20, 7, false, testColorBlue, testColorBlue)

	// Should contain all content lines
	assert.Contains(t, result, "Line 1", "missing Line 1")
	assert.Contains(t, result, "Line 2", "missing Line 2")
	assert.Contains(t, result, "Line 3", "missing Line 3")
}

func TestRenderWithTitleBorder_ContentPadding(t *testing.T) {
	result := RenderWithTitleBorder("Hi", "Title", 20, 5, false, testColorRed, testColorRed)

	lines := strings.Split(result, "\n")

	// Content lines (middle ones) should have consistent width
	// They should all be padded to the same width
	for i := 1; i < len(lines)-1; i++ {
		w := lipgloss.Width(lines[i])
		assert.Equal(t, 20, w, "line %d width %d, expected 20: %q", i, w, lines[i])
	}
}

func TestRenderWithTitleBorder_DifferentColors(t *testing.T) {
	// Test that different colors all render correctly
	colors := []struct {
		name  string
		color lipgloss.TerminalColor
	}{
		{"red", testColorRed},
		{"green", testColorGreen},
		{"blue", testColorBlue},
		{"purple", testColorPurple},
	}

	for _, tc := range colors {
		t.Run(tc.name, func(t *testing.T) {
			result := RenderWithTitleBorder("content", "Title", 20, 5, false, tc.color, tc.color)
			assert.Contains(t, result, "Title", "%s: missing title", tc.name)
			assert.Contains(t, result, "╭", "%s: missing border", tc.name)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{"fits", "Hello", 10, "Hello"},
		{"exact", "Hello", 5, "Hello"},
		{"truncate", "Hello World", 8, "Hello..."},
		{"very short", "Hello", 3, "..."},
		{"minimal", "Hello", 1, "."},
		{"zero", "Hello", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxWidth)
			assert.Equal(t, tt.want, got, "truncateString(%q, %d)", tt.input, tt.maxWidth)
		})
	}
}

func TestBuildTopBorder(t *testing.T) {
	borderStyle := lipgloss.NewStyle().Foreground(BorderDefaultColor)
	titleStyle := lipgloss.NewStyle().Foreground(testColorGreen)

	tests := []struct {
		name       string
		title      string
		innerWidth int
		wantTitle  bool
	}{
		{"normal", "Title", 20, true},
		{"empty title", "", 20, false},
		{"narrow", "Title", 3, false},
		{"just enough", "T", 6, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTopBorder(tt.title, tt.innerWidth, borderStyle, titleStyle)

			assert.True(t, strings.HasPrefix(got, "╭"), "should start with top-left corner")
			assert.True(t, strings.HasSuffix(got, "╮"), "should end with top-right corner")

			hasTitle := strings.Contains(got, tt.title) && tt.title != ""
			if tt.wantTitle {
				assert.True(t, hasTitle, "expected title %q in border: %s", tt.title, got)
			}
		})
	}
}
