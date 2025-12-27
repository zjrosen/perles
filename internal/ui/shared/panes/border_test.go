package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

// Test colors for bordered pane tests
var (
	testColorBlue   = lipgloss.AdaptiveColor{Light: "#54A0FF", Dark: "#54A0FF"}
	testColorGreen  = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}
	testColorPurple = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#7D56F4"}
)

// =============================================================================
// Unit Tests for BorderedPane
// =============================================================================

func TestBorderedPane_BasicRendering(t *testing.T) {
	cfg := BorderConfig{
		Content: "Hello World",
		Width:   20,
		Height:  5,
	}

	result := BorderedPane(cfg)

	// Should contain border characters (rounded by default)
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	require.Contains(t, result, "╰", "missing bottom-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
	require.Contains(t, result, "│", "missing vertical border")

	// Should contain content
	require.Contains(t, result, "Hello World", "missing content")

	// Should have correct line count
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 5, "expected 5 lines for height 5")
}

func TestBorderedPane_TopLeftTitle(t *testing.T) {
	cfg := BorderConfig{
		Content: "content",
		Width:   30,
		Height:  5,
		TopLeft: "My Title",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "My Title", "missing top-left title")
	require.Contains(t, result, "╭", "missing top-left corner")
}

func TestBorderedPane_TopRightTitle(t *testing.T) {
	cfg := BorderConfig{
		Content:  "content",
		Width:    30,
		Height:   5,
		TopRight: "Status",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Status", "missing top-right title")
}

func TestBorderedPane_BottomLeftTitle(t *testing.T) {
	cfg := BorderConfig{
		Content:    "content",
		Width:      30,
		Height:     5,
		BottomLeft: "Footer",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Footer", "missing bottom-left title")
	require.Contains(t, result, "╰", "missing bottom-left corner")
}

func TestBorderedPane_BottomRightTitle(t *testing.T) {
	cfg := BorderConfig{
		Content:     "content",
		Width:       30,
		Height:      5,
		BottomRight: "Page 1/5",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Page 1/5", "missing bottom-right title")
}

func TestBorderedPane_DualTitles(t *testing.T) {
	cfg := BorderConfig{
		Content:     "content",
		Width:       40,
		Height:      5,
		TopLeft:     "Title",
		TopRight:    "Info",
		BottomLeft:  "Help",
		BottomRight: "Status",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Title", "missing top-left title")
	require.Contains(t, result, "Info", "missing top-right title")
	require.Contains(t, result, "Help", "missing bottom-left title")
	require.Contains(t, result, "Status", "missing bottom-right title")
}

func TestBorderedPane_FocusedState(t *testing.T) {
	cfgUnfocused := BorderConfig{
		Content:            "content",
		Width:              20,
		Height:             5,
		TopLeft:            "Test",
		Focused:            false,
		BorderColor:        testColorBlue,
		FocusedBorderColor: testColorGreen,
	}

	cfgFocused := BorderConfig{
		Content:            "content",
		Width:              20,
		Height:             5,
		TopLeft:            "Test",
		Focused:            true,
		BorderColor:        testColorBlue,
		FocusedBorderColor: testColorGreen,
	}

	unfocusedResult := BorderedPane(cfgUnfocused)
	focusedResult := BorderedPane(cfgFocused)

	// Both should have valid structure
	require.Contains(t, unfocusedResult, "╭", "unfocused missing border")
	require.Contains(t, focusedResult, "╭", "focused missing border")
	require.Contains(t, unfocusedResult, "Test", "unfocused missing title")
	require.Contains(t, focusedResult, "Test", "focused missing title")

	// Results should have same line count but may differ in ANSI color codes
	unfocusedLines := strings.Split(unfocusedResult, "\n")
	focusedLines := strings.Split(focusedResult, "\n")
	require.Equal(t, len(unfocusedLines), len(focusedLines), "focused and unfocused should have same line count")
}

func TestBorderedPane_CustomColors(t *testing.T) {
	cfg := BorderConfig{
		Content:     "content",
		Width:       20,
		Height:      5,
		TopLeft:     "Test",
		TitleColor:  testColorPurple,
		BorderColor: testColorBlue,
	}

	result := BorderedPane(cfg)

	// Should render without error
	require.Contains(t, result, "Test", "missing title")
	require.Contains(t, result, "content", "missing content")
}

func TestBorderedPane_NilColors(t *testing.T) {
	// All nil colors should use defaults
	cfg := BorderConfig{
		Content:            "content",
		Width:              20,
		Height:             5,
		TopLeft:            "Test",
		TitleColor:         nil,
		BorderColor:        nil,
		FocusedBorderColor: nil,
	}

	result := BorderedPane(cfg)

	// Should render without error using defaults
	require.Contains(t, result, "Test", "missing title")
	require.Contains(t, result, "content", "missing content")
}

func TestBorderedPane_EmptyContent(t *testing.T) {
	cfg := BorderConfig{
		Content: "",
		Width:   20,
		Height:  5,
		TopLeft: "Empty",
	}

	result := BorderedPane(cfg)

	// Should still render valid border
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
	require.Contains(t, result, "Empty", "missing title")

	// Should have correct line count
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 5, "expected 5 lines for height 5")
}

func TestBorderedPane_NarrowWidth(t *testing.T) {
	cfg := BorderConfig{
		Content: "x",
		Width:   5,
		Height:  3,
	}

	result := BorderedPane(cfg)

	// Should render without panic
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 3, "expected 3 lines for height 3")
}

func TestBorderedPane_MinimumWidth(t *testing.T) {
	cfg := BorderConfig{
		Content: "x",
		Width:   3, // Minimum: just corners
		Height:  3,
	}

	result := BorderedPane(cfg)

	// Should render without panic even at minimum width
	require.NotEmpty(t, result, "result should not be empty")
}

func TestBorderedPane_SmallHeight(t *testing.T) {
	cfg := BorderConfig{
		Content: "content",
		Width:   20,
		Height:  3, // Minimum usable height
	}

	result := BorderedPane(cfg)

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 3, "expected 3 lines for height 3")
}

func TestBorderedPane_ContentTruncation(t *testing.T) {
	// Content wider than inner width should be truncated
	cfg := BorderConfig{
		Content: "This is a very long line that should be truncated to fit within the border",
		Width:   20,
		Height:  3,
	}

	result := BorderedPane(cfg)

	// Each line should fit within the width
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		require.LessOrEqual(t, lineWidth, 20, "line width exceeds border width")
	}
}

func TestBorderedPane_MultilineContent(t *testing.T) {
	cfg := BorderConfig{
		Content: "Line 1\nLine 2\nLine 3",
		Width:   20,
		Height:  5,
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Line 1", "missing line 1")
	require.Contains(t, result, "Line 2", "missing line 2")
	require.Contains(t, result, "Line 3", "missing line 3")
}

func TestBorderedPane_NoTitle(t *testing.T) {
	cfg := BorderConfig{
		Content: "content",
		Width:   20,
		Height:  5,
		// No titles set
	}

	result := BorderedPane(cfg)

	// Should render plain border without titles
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	require.Contains(t, result, "content", "missing content")

	// First line should be plain border (no title text between corners)
	lines := strings.Split(result, "\n")
	require.GreaterOrEqual(t, len(lines), 1, "expected at least 1 line")
}

// =============================================================================
// Unit Tests for resolveBorderColor
// =============================================================================

func TestResolveBorderColor_BothNil(t *testing.T) {
	// Both nil: should return default color
	result := resolveBorderColor(nil, nil, false)
	require.NotNil(t, result, "should return non-nil color")

	result = resolveBorderColor(nil, nil, true)
	require.NotNil(t, result, "should return non-nil color when focused")
}

func TestResolveBorderColor_OnlyBorderColor(t *testing.T) {
	// BorderColor set, FocusedBorderColor nil: inherit BorderColor for both states
	result := resolveBorderColor(testColorBlue, nil, false)
	require.Equal(t, testColorBlue, result, "unfocused should use BorderColor")

	result = resolveBorderColor(testColorBlue, nil, true)
	require.Equal(t, testColorBlue, result, "focused should inherit BorderColor")
}

func TestResolveBorderColor_OnlyFocusedBorderColor(t *testing.T) {
	// BorderColor nil, FocusedBorderColor set
	result := resolveBorderColor(nil, testColorGreen, false)
	require.NotNil(t, result, "unfocused should use default")

	result = resolveBorderColor(nil, testColorGreen, true)
	require.Equal(t, testColorGreen, result, "focused should use FocusedBorderColor")
}

func TestResolveBorderColor_BothSet(t *testing.T) {
	// Both set: use appropriately based on focused flag
	result := resolveBorderColor(testColorBlue, testColorGreen, false)
	require.Equal(t, testColorBlue, result, "unfocused should use BorderColor")

	result = resolveBorderColor(testColorBlue, testColorGreen, true)
	require.Equal(t, testColorGreen, result, "focused should use FocusedBorderColor")
}

// =============================================================================
// Unit Tests for buildTopBorder
// =============================================================================

func TestBuildTopBorder_EmptyTitle(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildTopBorder("", 10, borderStyle, titleStyle)

	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	require.Contains(t, result, "─", "missing horizontal border")
}

func TestBuildTopBorder_WithTitle(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildTopBorder("Test", 15, borderStyle, titleStyle)

	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	require.Contains(t, result, "Test", "missing title")
}

func TestBuildTopBorder_NarrowWidth(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	// Width too narrow for title
	result := buildTopBorder("Title", 3, borderStyle, titleStyle)

	// Should render border without title
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
}

func TestBuildTopBorder_ZeroWidth(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildTopBorder("Title", 0, borderStyle, titleStyle)

	// Should render minimal border
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
}

func TestBuildTopBorder_LongTitle(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	// Title longer than available space
	result := buildTopBorder("Very Long Title That Should Be Truncated", 15, borderStyle, titleStyle)

	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
	// Title should be truncated, full title should not appear
	require.NotContains(t, result, "Truncated", "title should be truncated")
}

// =============================================================================
// Unit Tests for buildBottomBorder
// =============================================================================

func TestBuildBottomBorder_EmptyTitle(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildBottomBorder("", 10, borderStyle, titleStyle)

	require.Contains(t, result, "╰", "missing bottom-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
	require.Contains(t, result, "─", "missing horizontal border")
}

func TestBuildBottomBorder_WithTitle(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildBottomBorder("Footer", 15, borderStyle, titleStyle)

	require.Contains(t, result, "╰", "missing bottom-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
	require.Contains(t, result, "Footer", "missing title")
}

// =============================================================================
// Unit Tests for buildDualTitleTopBorder
// =============================================================================

func TestBuildDualTitleTopBorder_BothEmpty(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleTopBorder("", "", 20, borderStyle, titleStyle)

	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
}

func TestBuildDualTitleTopBorder_LeftOnly(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleTopBorder("Left", "", 20, borderStyle, titleStyle)

	require.Contains(t, result, "Left", "missing left title")
	require.Contains(t, result, "╭", "missing top-left corner")
}

func TestBuildDualTitleTopBorder_RightOnly(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleTopBorder("", "Right", 20, borderStyle, titleStyle)

	require.Contains(t, result, "Right", "missing right title")
	require.Contains(t, result, "╮", "missing top-right corner")
}

func TestBuildDualTitleTopBorder_Both(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleTopBorder("Left", "Right", 30, borderStyle, titleStyle)

	require.Contains(t, result, "Left", "missing left title")
	require.Contains(t, result, "Right", "missing right title")
}

func TestBuildDualTitleTopBorder_TooNarrow(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	// Width too narrow for both titles
	result := buildDualTitleTopBorder("LeftTitle", "RightTitle", 10, borderStyle, titleStyle)

	// Should fall back to single title or plain border
	require.Contains(t, result, "╭", "missing top-left corner")
	require.Contains(t, result, "╮", "missing top-right corner")
}

// =============================================================================
// Unit Tests for buildDualTitleBottomBorder
// =============================================================================

func TestBuildDualTitleBottomBorder_BothEmpty(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleBottomBorder("", "", 20, borderStyle, titleStyle)

	require.Contains(t, result, "╰", "missing bottom-left corner")
	require.Contains(t, result, "╯", "missing bottom-right corner")
}

func TestBuildDualTitleBottomBorder_Both(t *testing.T) {
	borderStyle := lipgloss.NewStyle()
	titleStyle := lipgloss.NewStyle()

	result := buildDualTitleBottomBorder("Help", "Page 1", 30, borderStyle, titleStyle)

	require.Contains(t, result, "Help", "missing left title")
	require.Contains(t, result, "Page 1", "missing right title")
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestBorderedPane_WidthEqualsContentWidth(t *testing.T) {
	// Content exactly fills inner width
	cfg := BorderConfig{
		Content: "12345678", // 8 chars, with 2-char border = width 10
		Width:   10,
		Height:  3,
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "12345678", "content should be present")
}

func TestBorderedPane_UnicodeContent(t *testing.T) {
	cfg := BorderConfig{
		Content: "Hello 世界",
		Width:   20,
		Height:  3,
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "Hello", "missing English text")
	require.Contains(t, result, "世界", "missing Unicode content")
}

func TestBorderedPane_UnicodeTitle(t *testing.T) {
	cfg := BorderConfig{
		Content: "content",
		Width:   30,
		Height:  3,
		TopLeft: "日本語",
	}

	result := BorderedPane(cfg)

	require.Contains(t, result, "日本語", "missing Unicode title")
}

func TestBorderedPane_SpecialCharactersInContent(t *testing.T) {
	cfg := BorderConfig{
		Content: "Tab:\tNewline:\n<>&\"'",
		Width:   30,
		Height:  5,
	}

	result := BorderedPane(cfg)

	// Should render without panic
	require.Contains(t, result, "╭", "missing border")
}

// =============================================================================
// Golden Tests
// =============================================================================

func TestBorderedPane_Golden_Basic(t *testing.T) {
	cfg := BorderConfig{
		Content: "Hello World",
		Width:   30,
		Height:  5,
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_TopLeftTitle(t *testing.T) {
	cfg := BorderConfig{
		Content: "Some content here",
		Width:   40,
		Height:  5,
		TopLeft: "My Panel",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_AllTitles(t *testing.T) {
	cfg := BorderConfig{
		Content:     "Centered content",
		Width:       50,
		Height:      7,
		TopLeft:     "Title",
		TopRight:    "Status",
		BottomLeft:  "Help: ?",
		BottomRight: "1/10",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_MultilineContent(t *testing.T) {
	cfg := BorderConfig{
		Content: "Line 1: Hello\nLine 2: World\nLine 3: Test\nLine 4: Content",
		Width:   40,
		Height:  8,
		TopLeft: "Log Output",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_Narrow(t *testing.T) {
	cfg := BorderConfig{
		Content: "x",
		Width:   10,
		Height:  5,
		TopLeft: "N",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_Empty(t *testing.T) {
	cfg := BorderConfig{
		Content: "",
		Width:   30,
		Height:  5,
		TopLeft: "Empty",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_LongTitle(t *testing.T) {
	cfg := BorderConfig{
		Content: "content",
		Width:   30,
		Height:  5,
		TopLeft: "This is a very long title that exceeds the available width",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_DualTopTitles(t *testing.T) {
	cfg := BorderConfig{
		Content:  "Panel with dual top titles",
		Width:    50,
		Height:   5,
		TopLeft:  "Messages",
		TopRight: "12 unread",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}

func TestBorderedPane_Golden_DualBottomTitles(t *testing.T) {
	cfg := BorderConfig{
		Content:     "Panel with dual bottom titles",
		Width:       50,
		Height:      5,
		BottomLeft:  "Press ? for help",
		BottomRight: "Page 5/20",
	}

	result := BorderedPane(cfg)
	teatest.RequireEqualOutput(t, []byte(result))
}
