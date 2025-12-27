package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

// Test colors for scrollable pane tests
var (
	scrollTestColorBlue  = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	scrollTestColorGreen = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
)

// newTestViewport creates a new viewport with the given dimensions for testing.
func newTestViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	return vp
}

func TestScrollablePane_RendersContentCorrectly(t *testing.T) {
	vp := newTestViewport(18, 3)
	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Test",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
	}

	result := ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return "Hello World"
	})

	// Should contain border characters
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	require.Contains(t, result, "╰", "missing bottom-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")

	// Should contain title
	require.Contains(t, result, "Test", "missing title")

	// Should contain content
	require.Contains(t, result, "Hello World", "missing content")
}

func TestScrollablePane_ScrollIndicatorAppearsWhenOverflow(t *testing.T) {
	vp := newTestViewport(18, 3)

	// Set content that overflows the viewport
	longContent := strings.Repeat("line\n", 20)
	vp.SetContent(longContent)
	// Scroll up to trigger indicator
	vp.GotoTop()

	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Overflow",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
	}

	result := ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return longContent
	})

	// Should contain scroll indicator (↑ with percentage)
	require.Contains(t, result, "↑", "missing scroll indicator when scrolled up")
}

func TestScrollablePane_AutoScrollWhenAtBottom(t *testing.T) {
	vp := newTestViewport(18, 3)

	// Initial content
	vp.SetContent("line1\nline2\nline3")
	vp.GotoBottom() // User is at bottom

	cfg := ScrollableConfig{
		Viewport:     &vp,
		ContentDirty: true, // New content arrived
		LeftTitle:    "AutoScroll",
		TitleColor:   scrollTestColorBlue,
		BorderColor:  scrollTestColorBlue,
	}

	_ = ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return "line1\nline2\nline3\nline4\nline5\nline6\nline7"
	})

	// After render, viewport should still be at bottom (auto-scrolled)
	require.True(t, vp.AtBottom(), "viewport should auto-scroll to bottom when user was at bottom")
}

func TestScrollablePane_NoAutoScrollWhenScrolledUp(t *testing.T) {
	vp := newTestViewport(18, 3)

	// Set initial content and scroll up
	longContent := strings.Repeat("line\n", 20)
	vp.SetContent(longContent)
	vp.GotoTop() // User scrolled up

	initialYOffset := vp.YOffset

	cfg := ScrollableConfig{
		Viewport:     &vp,
		ContentDirty: true, // New content arrived
		LeftTitle:    "NoAutoScroll",
		TitleColor:   scrollTestColorBlue,
		BorderColor:  scrollTestColorBlue,
	}

	_ = ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return longContent + "new line\n"
	})

	// Viewport should NOT have scrolled to bottom (user was scrolled up)
	require.Equal(t, initialYOffset, vp.YOffset, "viewport should NOT auto-scroll when user was scrolled up")
}

func TestScrollablePane_HasNewContentShowsIndicator(t *testing.T) {
	vp := newTestViewport(18, 3)

	cfg := ScrollableConfig{
		Viewport:      &vp,
		HasNewContent: true, // New content indicator should show
		LeftTitle:     "NewContent",
		TitleColor:    scrollTestColorBlue,
		BorderColor:   scrollTestColorBlue,
	}

	result := ScrollablePane(30, 5, cfg, func(wrapWidth int) string {
		return "content"
	})

	// Should contain the new content indicator
	require.Contains(t, result, "↓New", "missing new content indicator")
}

func TestScrollablePane_MetricsDisplayInRightTitle(t *testing.T) {
	vp := newTestViewport(28, 3)

	cfg := ScrollableConfig{
		Viewport:       &vp,
		MetricsDisplay: "27k/200k",
		LeftTitle:      "Metrics",
		TitleColor:     scrollTestColorBlue,
		BorderColor:    scrollTestColorBlue,
	}

	result := ScrollablePane(30, 5, cfg, func(wrapWidth int) string {
		return "content"
	})

	// Should contain the metrics display
	require.Contains(t, result, "27k/200k", "missing metrics display")
}

func TestScrollablePane_ContentPaddingPushesToBottom(t *testing.T) {
	vp := newTestViewport(18, 5) // Larger viewport than content

	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Padding",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
	}

	_ = ScrollablePane(20, 7, cfg, func(wrapWidth int) string {
		return "short" // Only 1 line of content
	})

	// Content in viewport should have padding prepended
	content := vp.View()
	lines := strings.Split(content, "\n")

	// With height 7 and border 2, viewport is 5 lines
	// With 1 line of content, there should be 4 empty lines prepended
	require.GreaterOrEqual(t, len(lines), 5, "should have at least 5 lines with padding")

	// First lines should be empty (padding)
	emptyLineCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			emptyLineCount++
		} else {
			break // Stop at first non-empty line
		}
	}
	require.GreaterOrEqual(t, emptyLineCount, 1, "should have empty lines prepended for padding")
}

func TestScrollablePane_PointerSemanticsViewportModified(t *testing.T) {
	vp := newTestViewport(10, 3)
	originalYOffset := vp.YOffset

	// Set content that requires scrolling
	longContent := strings.Repeat("line\n", 20)

	cfg := ScrollableConfig{
		Viewport:     &vp,
		ContentDirty: true,
		LeftTitle:    "Pointer",
		TitleColor:   scrollTestColorBlue,
		BorderColor:  scrollTestColorBlue,
	}

	_ = ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return longContent
	})

	// Viewport should have been modified (dimensions updated, scrolled)
	// The YOffset should be different because GotoBottom was called
	require.NotEqual(t, originalYOffset, vp.YOffset, "viewport YOffset should be modified after auto-scroll")
}

func TestScrollablePane_FocusedStatePassedThrough(t *testing.T) {
	vp := newTestViewport(18, 3)

	cfgUnfocused := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Focus",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
		Focused:     false,
	}

	cfgFocused := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Focus",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
		Focused:     true,
	}

	unfocusedResult := ScrollablePane(20, 5, cfgUnfocused, func(wrapWidth int) string {
		return "content"
	})

	focusedResult := ScrollablePane(20, 5, cfgFocused, func(wrapWidth int) string {
		return "content"
	})

	// Both should have valid structure
	require.Contains(t, unfocusedResult, "╭", "unfocused missing border")
	require.Contains(t, focusedResult, "╭", "focused missing border")
	require.Contains(t, unfocusedResult, "Focus", "unfocused missing title")
	require.Contains(t, focusedResult, "Focus", "focused missing title")

	// Results may differ in styling (colors) but structure should be same
	unfocusedLines := strings.Split(unfocusedResult, "\n")
	focusedLines := strings.Split(focusedResult, "\n")
	require.Equal(t, len(unfocusedLines), len(focusedLines), "focused and unfocused should have same line count")
}

func TestScrollablePane_CompositionWithBorderedPane(t *testing.T) {
	vp := newTestViewport(18, 3)

	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Composition",
		TitleColor:  scrollTestColorGreen,
		BorderColor: scrollTestColorGreen,
		Focused:     true,
	}

	result := ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return "test content"
	})

	// Should have all the border characteristics
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "│", "missing vertical border")
	require.Contains(t, result, "╰", "missing bottom-left corner")

	// Should have the title
	require.Contains(t, result, "Composition", "missing title")

	// Should have the content
	require.Contains(t, result, "test content", "missing content")
}

func TestScrollablePane_EmptyContent(t *testing.T) {
	vp := newTestViewport(18, 3)

	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "Empty",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
	}

	result := ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return ""
	})

	// Should still render valid border
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
	require.Contains(t, result, "Empty", "missing title")

	// Should have correct line count (5 total)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 5, "expected 5 lines for height 5")
}

func TestScrollablePane_ContentExactlyFitsViewport(t *testing.T) {
	vp := newTestViewport(18, 3)

	// Height 5 - 2 borders = 3 lines of content
	cfg := ScrollableConfig{
		Viewport:    &vp,
		LeftTitle:   "ExactFit",
		TitleColor:  scrollTestColorBlue,
		BorderColor: scrollTestColorBlue,
	}

	result := ScrollablePane(20, 5, cfg, func(wrapWidth int) string {
		return "line1\nline2\nline3" // Exactly 3 lines
	})

	// Should render correctly with no scroll indicator (content fits)
	require.Contains(t, result, "line1", "missing line1")
	require.Contains(t, result, "line2", "missing line2")
	require.Contains(t, result, "line3", "missing line3")

	// Should NOT contain scroll indicator
	require.NotContains(t, result, "↑", "should not have scroll indicator when content fits")
}

// Test BuildScrollIndicator function directly

func TestBuildScrollIndicator_ContentFits(t *testing.T) {
	vp := newTestViewport(10, 5)
	vp.SetContent("line1\nline2\nline3") // 3 lines in 5-line viewport

	indicator := BuildScrollIndicator(vp)
	require.Empty(t, indicator, "should be empty when content fits viewport")
}

func TestBuildScrollIndicator_AtBottom(t *testing.T) {
	vp := newTestViewport(10, 3)
	vp.SetContent(strings.Repeat("line\n", 20)) // Content overflows
	vp.GotoBottom()

	indicator := BuildScrollIndicator(vp)
	require.Empty(t, indicator, "should be empty when at bottom (live view)")
}

func TestBuildScrollIndicator_ScrolledUp(t *testing.T) {
	vp := newTestViewport(10, 3)
	vp.SetContent(strings.Repeat("line\n", 20)) // Content overflows
	vp.GotoTop()

	indicator := BuildScrollIndicator(vp)
	require.NotEmpty(t, indicator, "should have indicator when scrolled up")
	require.Contains(t, indicator, "↑", "should contain up arrow")
	require.Contains(t, indicator, "%", "should contain percentage")
}
