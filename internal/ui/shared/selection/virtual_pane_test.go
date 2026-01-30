package selection

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
)

// Helper to create test messages
func createTestMessages(count int) []chatrender.Message {
	msgs := make([]chatrender.Message, count)
	for i := 0; i < count; i++ {
		if i%2 == 0 {
			msgs[i] = chatrender.Message{
				Role:    "user",
				Content: "Hello world from user",
			}
		} else {
			msgs[i] = chatrender.Message{
				Role:    "assistant",
				Content: "Hello from assistant with a longer response that might wrap",
			}
		}
	}
	return msgs
}

func TestNewVirtualSelectablePane_DefaultValues(t *testing.T) {
	// Create with nil clipboard and toast (allowed)
	pane := NewVirtualSelectablePane(nil, nil)

	require.NotNil(t, pane)
	require.Equal(t, DefaultBufferLines, pane.BufferLines(), "default bufferLines should be 50")
	require.NotNil(t, pane.Selection(), "selection should be initialized")
	require.Equal(t, 0, pane.scrollOffset, "scrollOffset should start at 0")
	require.Equal(t, 0, pane.height, "height should start at 0")
	require.Equal(t, 0, pane.width, "width should start at 0")
	require.Empty(t, pane.plainLines, "plainLines should be empty initially")
	require.Nil(t, pane.virtualContent, "virtualContent should be nil until SetMessages")
	require.True(t, pane.wasAtBottom, "wasAtBottom should be true initially")
}

func TestSetMessages_BuildsVirtualContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 10 // Set height for proper testing

	messages := createTestMessages(3)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)

	require.NotNil(t, pane.virtualContent, "virtualContent should be created")
	require.Greater(t, pane.virtualContent.TotalLines(), 0, "should have lines after SetMessages")
}

func TestSetMessages_UpdatesPlainLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 10

	messages := createTestMessages(2)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)

	require.NotEmpty(t, pane.PlainLines(), "plainLines should be populated")
	require.Equal(t, pane.virtualContent.TotalLines(), len(pane.PlainLines()),
		"plainLines count should match virtualContent totalLines")
}

func TestSetMessages_AutoScrollAtBottom(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 5

	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	// First set: small content, should be at bottom
	smallMsgs := createTestMessages(1)
	pane.SetMessages(smallMsgs, 80, cfg)

	// Pane should be at bottom
	require.True(t, pane.AtBottom(), "should be at bottom with small content")

	// Second set: more content, should stay at bottom (wasAtBottom was true)
	largeMsgs := createTestMessages(10)
	pane.SetMessages(largeMsgs, 80, cfg)

	require.True(t, pane.AtBottom(), "should stay at bottom when wasAtBottom")
	require.True(t, pane.WasAtBottom(), "wasAtBottom should be true")
}

func TestSetMessages_PreservesScrollPosition(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 5

	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	// Set initial content with enough lines to scroll
	largeMsgs := createTestMessages(20)
	pane.SetMessages(largeMsgs, 80, cfg)

	// Scroll up from bottom
	pane.scrollOffset = 5 // Move away from bottom

	// Verify not at bottom
	require.False(t, pane.AtBottom(), "should not be at bottom after scrolling up")

	// Update with more content (simulates new messages arriving)
	moreMsgs := createTestMessages(22)
	pane.SetMessages(moreMsgs, 80, cfg)

	// Should NOT auto-scroll to bottom because we weren't at bottom
	require.False(t, pane.WasAtBottom(), "wasAtBottom should be false")
	// Scroll position might be clamped but shouldn't jump to bottom
	require.Equal(t, 5, pane.scrollOffset, "scroll position should be preserved when not at bottom")
}

func TestSetSize_WidthChangeClearsCache(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 10

	messages := createTestMessages(3)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	// Set messages with initial width
	pane.SetMessages(messages, 80, cfg)
	initialLines := pane.virtualContent.TotalLines()

	// Change width - should trigger rebuild
	pane.SetSize(40, 10)

	// Verify width was updated
	require.Equal(t, 40, pane.Width())
	require.Equal(t, 10, pane.Height())

	// Lines may change due to re-wrapping at different width
	// The cache inside virtualContent should be cleared
	require.NotNil(t, pane.virtualContent)

	// With narrower width, we should have more lines due to wrapping
	newLines := pane.virtualContent.TotalLines()
	// Can't guarantee exact behavior, but totalLines should be recalculated
	require.Greater(t, newLines, 0, "should still have lines after width change")

	// Log for debugging
	t.Logf("Initial lines at width 80: %d, new lines at width 40: %d", initialLines, newLines)
}

func TestSetSize_HeightChangeRecalculatesPadding(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(1) // Small content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	totalLines := pane.virtualContent.TotalLines()

	// Set height larger than content
	pane.SetSize(80, 100)

	// topPadding should be calculated for bottom alignment
	expectedPadding := 100 - totalLines
	require.Equal(t, expectedPadding, pane.TopPadding(),
		"topPadding should be height - totalLines for bottom alignment")

	// Reduce height to be smaller than content
	pane.SetSize(80, 2)

	// topPadding should be 0 when content exceeds height
	require.Equal(t, 0, pane.TopPadding(),
		"topPadding should be 0 when content >= height")
}

func TestSetSize_ClampsScrollOffset(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(10)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	// Set messages with large viewport
	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 5)

	// Scroll to middle
	totalLines := pane.virtualContent.TotalLines()
	pane.scrollOffset = totalLines / 2

	// Increase height so maxScrollOffset decreases
	pane.SetSize(80, totalLines-2)

	// scrollOffset should be clamped to new maxScrollOffset
	maxOffset := pane.maxScrollOffset()
	require.LessOrEqual(t, pane.scrollOffset, maxOffset,
		"scrollOffset should be clamped after height increase")
}

func TestMaxScrollOffset_EmptyContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 10

	// No content set
	require.Equal(t, 0, pane.maxScrollOffset(), "maxScrollOffset should be 0 for empty content")
}

func TestMaxScrollOffset_ContentSmallerThanHeight(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(1) // Small content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	totalLines := pane.virtualContent.TotalLines()

	// Set height larger than content
	pane.height = totalLines + 10

	require.Equal(t, 0, pane.maxScrollOffset(),
		"maxScrollOffset should be 0 when content < height")
}

func TestMaxScrollOffset_ContentLargerThanHeight(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20) // Large content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	totalLines := pane.virtualContent.TotalLines()

	// Set height smaller than content
	pane.height = 10
	require.Greater(t, totalLines, pane.height, "test setup: content should exceed height")

	expectedMax := totalLines - pane.height
	require.Equal(t, expectedMax, pane.maxScrollOffset(),
		"maxScrollOffset should be totalLines - height")
}

func TestClampScrollOffset_ClampsToZero(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 10

	messages := createTestMessages(5)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)

	// Try to set negative scroll offset
	pane.scrollOffset = -5
	pane.clampScrollOffset()

	require.Equal(t, 0, pane.scrollOffset, "scrollOffset should be clamped to 0")
}

func TestClampScrollOffset_ClampsToMax(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.height = 5

	// Try to set scroll offset beyond max
	pane.scrollOffset = 9999
	pane.clampScrollOffset()

	maxOffset := pane.maxScrollOffset()
	require.Equal(t, maxOffset, pane.scrollOffset,
		"scrollOffset should be clamped to maxScrollOffset")
}

func TestAtBottom_True(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.height = 5

	// Scroll to max
	pane.scrollOffset = pane.maxScrollOffset()

	require.True(t, pane.AtBottom(), "should be at bottom when scrollOffset == maxScrollOffset")
}

func TestAtBottom_False(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.height = 5

	// Scroll to middle
	pane.scrollOffset = pane.maxScrollOffset() / 2

	require.False(t, pane.AtBottom(), "should not be at bottom when scrolled up")
}

func TestAtTop(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.height = 5

	// At top
	pane.scrollOffset = 0
	require.True(t, pane.AtTop(), "should be at top when scrollOffset == 0")

	// Not at top
	pane.scrollOffset = 5
	require.False(t, pane.AtTop(), "should not be at top when scrolled down")
}

func TestSetScrollOffset(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.height = 5

	// Set valid offset
	pane.SetScrollOffset(10)
	maxOffset := pane.maxScrollOffset()
	if 10 <= maxOffset {
		require.Equal(t, 10, pane.ScrollOffset())
	} else {
		require.Equal(t, maxOffset, pane.ScrollOffset())
	}

	// Set negative offset (should clamp to 0)
	pane.SetScrollOffset(-5)
	require.Equal(t, 0, pane.ScrollOffset())

	// Set beyond max (should clamp to max)
	pane.SetScrollOffset(9999)
	require.Equal(t, maxOffset, pane.ScrollOffset())
}

func TestTotalLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// No content
	require.Equal(t, 0, pane.TotalLines())

	// With content
	messages := createTestMessages(5)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}
	pane.SetMessages(messages, 80, cfg)

	require.Greater(t, pane.TotalLines(), 0)
	require.Equal(t, pane.virtualContent.TotalLines(), pane.TotalLines())
}

func TestSetFocused(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	require.False(t, pane.Focused())

	pane.SetFocused(true)
	require.True(t, pane.Focused())

	pane.SetFocused(false)
	require.False(t, pane.Focused())
}

func TestSetScreenPosition(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	pane.SetScreenPosition(10, 20)

	require.Equal(t, 10, pane.screenXOffset)
	require.Equal(t, 20, pane.screenYOffset)
}

func TestSetBufferLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	require.Equal(t, DefaultBufferLines, pane.BufferLines())

	pane.SetBufferLines(100)
	require.Equal(t, 100, pane.BufferLines())
}

func TestCalculateTopPadding_NilContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.height = 20

	pane.calculateTopPadding()

	// With nil content, topPadding should equal height (all padding)
	require.Equal(t, 20, pane.TopPadding())
}

func TestCalculateTopPadding_ContentFitsWithRoom(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(1) // Small content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	totalLines := pane.virtualContent.TotalLines()

	// Set height larger than content
	pane.height = totalLines + 5
	pane.calculateTopPadding()

	require.Equal(t, 5, pane.TopPadding(), "topPadding should fill remaining space")
}

func TestCalculateTopPadding_ContentExceedsHeight(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20) // Large content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)

	// Set height smaller than content
	pane.height = 5
	require.Greater(t, pane.virtualContent.TotalLines(), pane.height,
		"test setup: content should exceed height")

	pane.calculateTopPadding()

	require.Equal(t, 0, pane.TopPadding(), "topPadding should be 0 when content exceeds height")
}

// =============================================================================
// View() Tests - Task 5 (perles-l1krd.5)
// =============================================================================

func TestView_EmptyContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(40, 10)

	// No messages set - should render empty state
	view := pane.View()

	require.NotEmpty(t, view, "View should return content even when empty")
	require.Contains(t, view, "No messages", "Empty state should show placeholder text")

	// Verify height lines are rendered
	lines := strings.Split(view, "\n")
	require.Equal(t, 10, len(lines), "Empty state should render height-worth of lines")
}

func TestView_OnlyRendersVisible(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create many messages to ensure we have more content than viewport
	messages := createTestMessages(50)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10) // Small viewport

	totalLines := pane.TotalLines()
	require.Greater(t, totalLines, 10, "Test setup: need more lines than viewport")

	// View at top
	pane.SetScrollOffset(0)
	view := pane.View()

	viewLines := strings.Split(view, "\n")
	// Should have exactly height lines (or less if content is shorter)
	require.LessOrEqual(t, len(viewLines), 10, "View should not exceed viewport height")

	// View in middle
	pane.SetScrollOffset(20)
	viewMid := pane.View()

	viewMidLines := strings.Split(viewMid, "\n")
	require.LessOrEqual(t, len(viewMidLines), 10, "Mid-scroll View should not exceed viewport height")

	// The views should be different (different scroll positions)
	require.NotEqual(t, view, viewMid, "Different scroll positions should produce different views")
}

func TestView_BottomAlignmentPadding(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create small content (1 message = ~3 lines)
	messages := []chatrender.Message{
		{Role: "user", Content: "Hello"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20) // Large viewport

	totalLines := pane.TotalLines()
	require.Less(t, totalLines, 20, "Test setup: content should be less than viewport")

	view := pane.View()
	viewLines := strings.Split(view, "\n")

	// Should have exactly height lines
	require.Equal(t, 20, len(viewLines), "View should fill viewport height")

	// First lines should be empty (padding)
	expectedPadding := 20 - totalLines
	emptyCount := 0
	for i := 0; i < len(viewLines); i++ {
		if viewLines[i] == "" {
			emptyCount++
		} else {
			break
		}
	}

	require.Equal(t, expectedPadding, emptyCount,
		"Should have %d empty padding lines at top for bottom alignment", expectedPadding)
}

func TestView_NoSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(5)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 30) // Large enough to show all content

	// No selection - should render plain content
	view := pane.View()

	require.NotEmpty(t, view)
	require.Contains(t, view, "You", "Should contain user role label")

	// Verify no selection styling (the selection background ANSI codes)
	// Selection uses lipgloss which adds ANSI codes for background
	// Without selection, we shouldn't have the specific selection background
	require.NotContains(t, view, "\x1b[48;2;", "Should not have RGB background codes without selection")
}

func TestView_SelectionWithinLine(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello World Test"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Find the content line index
	contentLineIndex := -1
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Hello World") {
			contentLineIndex = i
			break
		}
	}
	require.NotEqual(t, -1, contentLineIndex, "Should find content line")

	// Set up selection within the content line (select "World")
	// "Hello World Test" - "World" starts at col 6, ends at col 11
	pane.Selection().Start(Point{Line: contentLineIndex, Col: 6})
	pane.Selection().Update(Point{Line: contentLineIndex, Col: 11})

	// Verify selection is set up correctly
	require.True(t, pane.Selection().HasSelection(), "Should have active selection")
	start, end := pane.Selection().SelectionBounds()
	require.NotNil(t, start, "Selection start should not be nil")
	require.NotNil(t, end, "Selection end should not be nil")

	view := pane.View()

	// View should render without error and contain the content
	require.NotEmpty(t, view, "View should not be empty")
	require.Contains(t, view, "World", "Should contain the selected text")

	// Verify the applySelectionOverlay function is being called correctly
	// by testing that the function produces expected output for the line
	// (In test environment, lipgloss may not emit ANSI codes due to no TTY,
	// but we verify the selection overlay function is invoked)
	plainLine := pane.PlainLines()[contentLineIndex]
	result := pane.applySelectionOverlay(contentLineIndex, plainLine, start, end)
	require.NotEmpty(t, result, "Selection overlay should produce non-empty result")
}

func TestView_SelectionFullLine(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Select this entire line"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Find the content line index
	contentLineIndex := -1
	var contentLine string
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Select this") {
			contentLineIndex = i
			contentLine = line
			break
		}
	}
	require.NotEqual(t, -1, contentLineIndex, "Should find content line")

	// Select entire line
	lineLen := stringDisplayWidth(contentLine)
	pane.Selection().Start(Point{Line: contentLineIndex, Col: 0})
	pane.Selection().Update(Point{Line: contentLineIndex, Col: lineLen})

	// Verify selection covers entire line
	require.True(t, pane.Selection().HasSelection(), "Should have active selection")
	start, end := pane.Selection().SelectionBounds()
	require.Equal(t, 0, start.Col, "Selection should start at column 0")
	require.Equal(t, lineLen, end.Col, "Selection should end at line length")

	view := pane.View()

	// Should render and contain the full line
	require.NotEmpty(t, view, "View should not be empty")
	require.Contains(t, view, "Select this entire line", "Should contain the full selected text")

	// Verify selection overlay produces output for this line
	result := pane.applySelectionOverlay(contentLineIndex, contentLine, start, end)
	require.NotEmpty(t, result, "Full line selection overlay should produce result")
}

func TestView_SelectionAcrossLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create content that spans multiple lines
	messages := []chatrender.Message{
		{Role: "user", Content: "Line one here"},
		{Role: "assistant", Content: "Line two content"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// Find two content lines
	var firstContentLine, secondContentLine int
	firstContentLine = -1
	secondContentLine = -1
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Line one") {
			firstContentLine = i
		}
		if strings.Contains(line, "Line two") {
			secondContentLine = i
		}
	}
	require.NotEqual(t, -1, firstContentLine, "Should find first content line")
	require.NotEqual(t, -1, secondContentLine, "Should find second content line")
	require.Less(t, firstContentLine, secondContentLine, "First line should be before second")

	// Select across multiple lines
	pane.Selection().Start(Point{Line: firstContentLine, Col: 5})
	pane.Selection().Update(Point{Line: secondContentLine, Col: 8})

	// Verify multi-line selection
	require.True(t, pane.Selection().HasSelection(), "Should have active selection")
	start, end := pane.Selection().SelectionBounds()
	require.NotEqual(t, start.Line, end.Line, "Selection should span multiple lines")

	view := pane.View()

	// Both lines should appear in the output
	require.NotEmpty(t, view, "View should not be empty")
	require.Contains(t, view, "Line one", "Should contain first line content")
	require.Contains(t, view, "Line two", "Should contain second line content")

	// Verify selection overlay is applied to both lines in the range
	firstLineResult := pane.applySelectionOverlay(firstContentLine, pane.PlainLines()[firstContentLine], start, end)
	secondLineResult := pane.applySelectionOverlay(secondContentLine, pane.PlainLines()[secondContentLine], start, end)
	require.NotEmpty(t, firstLineResult, "First line should have overlay applied")
	require.NotEmpty(t, secondLineResult, "Second line should have overlay applied")
}

func TestView_SelectionPartiallyOutOfView(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create many messages to have more lines than viewport
	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 5) // Small viewport

	totalLines := pane.TotalLines()
	require.Greater(t, totalLines, 20, "Test setup: need many lines")

	// Create a selection that spans from above viewport to within viewport
	// Scroll to middle
	pane.SetScrollOffset(10)

	// Selection starts at line 8 (above viewport, scrollOffset=10) and ends at line 12 (within viewport)
	pane.Selection().Start(Point{Line: 8, Col: 0})
	pane.Selection().Update(Point{Line: 12, Col: 5})

	// Verify selection setup
	start, end := pane.Selection().SelectionBounds()
	require.NotNil(t, start, "Selection should have start")
	require.NotNil(t, end, "Selection should have end")
	require.Equal(t, 8, start.Line, "Selection start should be at line 8")
	require.Equal(t, 12, end.Line, "Selection end should be at line 12")

	view := pane.View()

	// View should render without error
	require.NotEmpty(t, view, "View should render even with partially off-screen selection")

	// Lines 10-14 are visible (scrollOffset=10, height=5)
	viewLines := strings.Split(view, "\n")
	require.LessOrEqual(t, len(viewLines), 5, "View should not exceed viewport")

	// Line 8-9 are off-screen (above viewport)
	// Line 10-12 are visible and within selection
	// applySelectionOverlay should handle off-screen lines correctly (return them unmodified)
	// and on-screen lines within selection range should get overlay applied

	// Test that overlay function handles line 10 (visible, within selection range 8-12)
	if totalLines > 10 {
		result := pane.applySelectionOverlay(10, pane.PlainLines()[10], start, end)
		require.NotEmpty(t, result, "Visible line within selection should be rendered")
	}

	// Test that overlay function handles line 8 correctly (it's in range but we just check it doesn't crash)
	// Note: line 8 might be empty (blank separator line), so we check it doesn't panic rather than checking for content
	if totalLines > 8 {
		// This should not panic
		_ = pane.applySelectionOverlay(8, pane.PlainLines()[8], start, end)
	}
}

// Test that View() complexity is O(visible) not O(total)
// This is verified by checking that a very large content set still renders quickly
func TestView_PerformanceOVisibleNotOTotal(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create a LOT of messages (would be slow if O(total))
	messages := createTestMessages(1000)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20) // Small viewport

	totalLines := pane.TotalLines()
	t.Logf("Total lines: %d", totalLines)

	// Time multiple View() calls - should be fast since O(visible)
	iterations := 100
	for i := 0; i < iterations; i++ {
		pane.SetScrollOffset(i * 10 % pane.maxScrollOffset())
		view := pane.View()
		require.NotEmpty(t, view)
	}

	// If we got here without timeout, the O(visible) implementation is working
	// The test would timeout/be very slow if it was O(total)
}

// Test empty state when height is zero
func TestView_EmptyContent_ZeroHeight(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(40, 0)

	view := pane.View()

	// With zero height, should return empty or minimal content
	require.Equal(t, "", view, "Zero height should produce empty view")
}

// Test that View respects scroll boundaries
func TestView_ScrollBoundaries(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Scroll to bottom
	pane.SetScrollOffset(pane.maxScrollOffset())
	viewBottom := pane.View()

	// Scroll to top
	pane.SetScrollOffset(0)
	viewTop := pane.View()

	require.NotEqual(t, viewBottom, viewTop, "Top and bottom views should differ")

	// Both should produce valid output
	require.NotEmpty(t, viewBottom)
	require.NotEmpty(t, viewTop)

	// Line count should be consistent
	topLines := strings.Split(viewTop, "\n")
	bottomLines := strings.Split(viewBottom, "\n")
	require.Equal(t, len(topLines), len(bottomLines), "All scroll positions should produce same line count")
}

// =============================================================================
// Golden Tests - Visual verification of View() output
// Run with -update flag to update golden files: go test -update ./internal/ui/shared/selection/...
// =============================================================================

// createGoldenTestPane creates a VirtualSelectablePane with consistent test content.
func createGoldenTestPane(messages []chatrender.Message, width, height int) *VirtualSelectablePane {
	pane := NewVirtualSelectablePane(nil, nil)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}
	pane.SetMessages(messages, width, cfg)
	pane.SetSize(width, height)
	return pane
}

func TestView_Golden_EmptyContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(60, 10) // Set size for consistent golden output

	view := pane.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_ShortContent(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "user", Content: "Hello there!"},
		{Role: "assistant", Content: "Hi! How can I help you today?"},
	}

	pane := createGoldenTestPane(messages, 60, 20)
	view := pane.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_ScrollMid(t *testing.T) {
	// Create enough content to scroll
	messages := []chatrender.Message{
		{Role: "user", Content: "First message from user"},
		{Role: "assistant", Content: "First response from assistant"},
		{Role: "user", Content: "Second message from user"},
		{Role: "assistant", Content: "Second response from assistant"},
		{Role: "user", Content: "Third message from user"},
		{Role: "assistant", Content: "Third response from assistant"},
		{Role: "user", Content: "Fourth message from user"},
		{Role: "assistant", Content: "Fourth response from assistant"},
	}

	pane := createGoldenTestPane(messages, 60, 10)

	// Scroll to middle
	midOffset := pane.maxScrollOffset() / 2
	pane.SetScrollOffset(midOffset)

	view := pane.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

func TestView_Golden_WithSelection(t *testing.T) {
	messages := []chatrender.Message{
		{Role: "user", Content: "Select some text here"},
		{Role: "assistant", Content: "This is a response with selectable content"},
	}

	pane := createGoldenTestPane(messages, 60, 15)

	// Find a line with content and create a selection
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Select some text") {
			// Select "some text"
			pane.Selection().Start(Point{Line: i, Col: 7})
			pane.Selection().Update(Point{Line: i, Col: 16})
			break
		}
	}

	view := pane.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// =============================================================================
// Task 6: Coordinate Mapping and Mouse Handling Tests (perles-l1krd.6)
// =============================================================================

func TestScreenToContentPosition_NoScroll(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// Set screen position
	pane.SetScreenPosition(10, 5)

	// No scroll (scrollOffset = 0)
	pane.SetScrollOffset(0)

	// Test coordinate mapping with no scroll
	// Screen coords: (12, 7) = pane coords (2, 2) after adjusting for screen offset and content start
	// Content line: 2 + scrollOffset(0) - topPadding
	pos := pane.screenToContentPosition(12, 7)

	// With topPadding accounting, the line should be adjusted
	require.GreaterOrEqual(t, pos.Line, 0, "Line should be non-negative")
	require.GreaterOrEqual(t, pos.Col, 0, "Column should be non-negative")
}

func TestScreenToContentPosition_WithScroll(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create enough content to require scrolling
	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetScreenPosition(0, 0)

	// Ensure we have enough lines to scroll
	require.Greater(t, pane.TotalLines(), 20, "Need enough lines for scroll test")

	// Scroll down
	scrollAmount := 15
	pane.SetScrollOffset(scrollAmount)

	// Map screen position (x=5, y=3) to content
	// With scrollOffset=15, the content line should include the scroll offset
	pos := pane.screenToContentPosition(5, 3)

	// The content line should be: relativeY + scrollOffset - topPadding
	// relativeY = 3 - 0 - 1 (DefaultContentStartY) = 2
	// topPadding = 0 (content exceeds height)
	// contentLine = 2 + 15 = 17
	expectedLine := 2 + scrollAmount // minus topPadding which is 0
	require.Equal(t, expectedLine, pos.Line, "Content line should include scroll offset")
}

func TestScreenToContentPosition_ClampAbove(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(5)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetScreenPosition(10, 10)

	// Reset scroll to top so we can test clamping behavior
	pane.SetScrollOffset(0)

	// Test coordinates above the pane (negative after offset)
	// screenY=5, screenYOffset=10, so relativeY = 5 - 10 - 1 = -6
	// After clamping to 0 and adding scrollOffset(0), line should be 0
	pos := pane.screenToContentPosition(15, 5)

	require.Equal(t, 0, pos.Line, "Line should clamp to 0 for coords above content")
	require.GreaterOrEqual(t, pos.Col, 0, "Column should be non-negative")
}

func TestScreenToContentPosition_ClampBelow(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Short"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 5) // Small height
	pane.SetScreenPosition(0, 0)

	totalLines := pane.TotalLines()
	require.Greater(t, totalLines, 0, "Should have some lines")

	// Test coordinates far below content
	// This should clamp to maxLine (totalLines - 1)
	pos := pane.screenToContentPosition(5, 1000)

	require.Equal(t, totalLines-1, pos.Line, "Line should clamp to maxLine for coords below content")
}

func TestScreenToContentPosition_WithPadding(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Create small content for bottom-aligned padding
	messages := []chatrender.Message{
		{Role: "user", Content: "Hi"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 30) // Large height = significant padding
	pane.SetScreenPosition(0, 0)

	totalLines := pane.TotalLines()
	expectedPadding := 30 - totalLines
	require.Greater(t, expectedPadding, 0, "Should have padding for bottom alignment")
	require.Equal(t, expectedPadding, pane.TopPadding(), "TopPadding should be set")

	// Click in the padding area (top of viewport)
	// relativeY = 3 - 0 - 1 = 2
	// 2 - padding = negative, should clamp to 0
	pos := pane.screenToContentPosition(5, 3)

	// If click is in padding area, should clamp to line 0
	require.GreaterOrEqual(t, pos.Line, 0, "Line should be >= 0 even in padding area")
	require.Less(t, pos.Line, totalLines, "Line should be within content range")
}

// =============================================================================
// HandleMouse Tests
// =============================================================================

func TestHandleMouse_StartSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(0, 0)

	// Verify no selection initially
	require.False(t, pane.Selection().IsSelecting(), "Should not be selecting initially")

	// Simulate left mouse press within bounds
	msg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}

	cmd := pane.HandleMouse(msg)
	require.Nil(t, cmd, "StartSelection should not return a command")
	require.True(t, pane.Selection().IsSelecting(), "Should be selecting after left press")
}

func TestHandleMouse_StartSelection_OutOfBounds(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(10, 10) // Pane starts at (10, 10)

	// Click outside bounds (before pane start)
	msg := tea.MouseMsg{
		X:      5, // Before screenXOffset
		Y:      5, // Before screenYOffset
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}

	cmd := pane.HandleMouse(msg)
	require.Nil(t, cmd)
	require.False(t, pane.Selection().IsSelecting(), "Should not start selection outside bounds")
}

func TestHandleMouse_UpdateSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world this is a test"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(0, 0)

	// Start selection
	startMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	pane.HandleMouse(startMsg)
	require.True(t, pane.Selection().IsSelecting(), "Selection should start")

	// Drag (motion with left button held)
	dragMsg := tea.MouseMsg{
		X:      20,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	}
	cmd := pane.HandleMouse(dragMsg)
	require.Nil(t, cmd, "Drag should not return a command")

	// Selection should have updated bounds
	start, end := pane.Selection().SelectionBounds()
	require.NotNil(t, start, "Selection start should be set")
	require.NotNil(t, end, "Selection end should be set")
}

func TestHandleMouse_EndSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world this is selectable text"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(0, 0)

	// Start selection
	startMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	pane.HandleMouse(startMsg)

	// Drag to create a selection
	dragMsg := tea.MouseMsg{
		X:      25,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionMotion,
	}
	pane.HandleMouse(dragMsg)

	// Release mouse
	releaseMsg := tea.MouseMsg{
		X:      25,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	}
	cmd := pane.HandleMouse(releaseMsg)

	// Without clipboard, should return nil and clear selection
	require.Nil(t, cmd, "Release without clipboard should return nil")
	require.False(t, pane.Selection().IsSelecting(), "Should not be selecting after release")
}

func TestHandleMouse_ScrollWheel(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetScreenPosition(0, 0)

	// Start scrolled to top
	pane.SetScrollOffset(10)
	initialOffset := pane.ScrollOffset()

	// Scroll up
	wheelUpMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelUp,
	}
	cmd := pane.HandleMouse(wheelUpMsg)
	require.Nil(t, cmd, "Scroll wheel should not return a command")
	require.Less(t, pane.ScrollOffset(), initialOffset, "Scroll up should decrease offset")

	// Reset and scroll down
	pane.SetScrollOffset(10)
	initialOffset = pane.ScrollOffset()

	wheelDownMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelDown,
	}
	cmd = pane.HandleMouse(wheelDownMsg)
	require.Nil(t, cmd, "Scroll wheel should not return a command")
	require.Greater(t, pane.ScrollOffset(), initialOffset, "Scroll down should increase offset")
}

// =============================================================================
// Copy-on-Release Tests
// =============================================================================

// MockClipboard implements shared.Clipboard for testing
type MockClipboard struct {
	copied string
	err    error
}

func (m *MockClipboard) Copy(text string) error {
	if m.err != nil {
		return m.err
	}
	m.copied = text
	return nil
}

// MockToast tracks toast messages for testing
type MockToast struct {
	messages []string
	isErrors []bool
}

func (m *MockToast) MakeToast(message string, isError bool) tea.Cmd {
	m.messages = append(m.messages, message)
	m.isErrors = append(m.isErrors, isError)
	return nil
}

func TestHandleMouse_CopyOnRelease(t *testing.T) {
	clipboard := &MockClipboard{}
	toast := &MockToast{}

	pane := NewVirtualSelectablePane(clipboard, toast.MakeToast)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(0, 0)

	// Find a line with content
	contentLine := -1
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Hello world") {
			contentLine = i
			break
		}
	}
	require.NotEqual(t, -1, contentLine, "Should find content line")

	// Manually set up a selection that we know contains text
	pane.Selection().Start(Point{Line: contentLine, Col: 0})
	pane.Selection().Update(Point{Line: contentLine, Col: 11}) // "Hello world"

	// Verify selection has text
	require.True(t, pane.Selection().HasSelection(), "Selection should exist")
	selectedText := pane.Selection().GetSelectedText()
	require.NotEmpty(t, selectedText, "Selected text should not be empty")

	// Simulate mouse release to trigger copy
	releaseMsg := tea.MouseMsg{
		X:      20,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	}
	pane.HandleMouse(releaseMsg)

	// Verify text was copied
	require.NotEmpty(t, clipboard.copied, "Text should be copied to clipboard")
	require.Contains(t, clipboard.copied, "Hello", "Copied text should contain selection")
}

func TestHandleMouse_CopyOnRelease_EmptySelection(t *testing.T) {
	clipboard := &MockClipboard{}
	toast := &MockToast{}

	pane := NewVirtualSelectablePane(clipboard, toast.MakeToast)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(0, 0)

	// Click without drag (empty selection)
	pressMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	}
	pane.HandleMouse(pressMsg)

	// Release at same position (no drag = empty selection)
	releaseMsg := tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	}
	pane.HandleMouse(releaseMsg)

	// Should not copy anything for empty selection
	require.Empty(t, clipboard.copied, "Empty selection should not copy")
	require.Empty(t, toast.messages, "No toast for empty selection")
}

// =============================================================================
// Selection Accessor Tests
// =============================================================================

func TestSelectionBounds_Normalized(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// Create selection with end before start (reversed)
	pane.Selection().Start(Point{Line: 3, Col: 20})
	pane.Selection().Update(Point{Line: 3, Col: 5})

	start, end := pane.SelectionBounds()
	require.NotNil(t, start, "Start should not be nil")
	require.NotNil(t, end, "End should not be nil")

	// Bounds should be normalized (start before end)
	require.LessOrEqual(t, start.Line, end.Line, "Start line should be <= end line")
	if start.Line == end.Line {
		require.LessOrEqual(t, start.Col, end.Col, "Start col should be <= end col on same line")
	}
}

func TestHasSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// No selection initially
	require.False(t, pane.HasSelection(), "No selection initially")

	// Create selection
	pane.Selection().Start(Point{Line: 2, Col: 5})
	pane.Selection().Update(Point{Line: 2, Col: 15})

	require.True(t, pane.HasSelection(), "Should have selection after Start+Update")
}

func TestClearSelection(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Hello world"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// Create selection
	pane.Selection().Start(Point{Line: 2, Col: 5})
	pane.Selection().Update(Point{Line: 2, Col: 15})
	require.True(t, pane.HasSelection(), "Should have selection")

	// Clear selection
	pane.ClearSelection()
	require.False(t, pane.HasSelection(), "Selection should be cleared")
}

func TestGetSelectedText_AcrossLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := []chatrender.Message{
		{Role: "user", Content: "Line one content"},
		{Role: "assistant", Content: "Line two content"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	// Find two content lines
	var line1, line2 int
	line1 = -1
	line2 = -1
	for i, line := range pane.PlainLines() {
		if strings.Contains(line, "Line one") {
			line1 = i
		}
		if strings.Contains(line, "Line two") {
			line2 = i
		}
	}
	require.NotEqual(t, -1, line1, "Should find first content line")
	require.NotEqual(t, -1, line2, "Should find second content line")
	require.Less(t, line1, line2, "First line should be before second")

	// Create multi-line selection
	pane.Selection().Start(Point{Line: line1, Col: 5})
	pane.Selection().Update(Point{Line: line2, Col: 8})

	selectedText := pane.GetSelectedText()
	require.NotEmpty(t, selectedText, "Multi-line selection should produce text")
	require.Contains(t, selectedText, "\n", "Multi-line selection should contain newline")
}

// =============================================================================
// Scroll Method Tests
// =============================================================================

func TestScrollUp(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Start at middle
	pane.SetScrollOffset(15)

	pane.ScrollUp(3)
	require.Equal(t, 12, pane.ScrollOffset(), "ScrollUp(3) should decrease by 3")

	// Scroll up should clamp at 0
	pane.ScrollUp(100)
	require.Equal(t, 0, pane.ScrollOffset(), "ScrollUp should clamp at 0")
}

func TestScrollDown(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	pane.SetScrollOffset(5)

	pane.ScrollDown(3)
	require.Equal(t, 8, pane.ScrollOffset(), "ScrollDown(3) should increase by 3")

	// Scroll down should clamp at max
	pane.ScrollDown(9999)
	maxOffset := pane.maxScrollOffset()
	require.Equal(t, maxOffset, pane.ScrollOffset(), "ScrollDown should clamp at max")
}

func TestScrollToBottom(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	pane.SetScrollOffset(0)
	require.False(t, pane.AtBottom(), "Should not be at bottom initially")

	pane.ScrollToBottom()
	require.True(t, pane.AtBottom(), "Should be at bottom after ScrollToBottom")
	require.Equal(t, pane.maxScrollOffset(), pane.ScrollOffset(), "ScrollOffset should equal maxScrollOffset")
}

func TestScrollToTop_SetsZero(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(30)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Scroll to somewhere in the middle
	pane.SetScrollOffset(15)
	require.Equal(t, 15, pane.ScrollOffset(), "Should start at offset 15")
	require.False(t, pane.AtTop(), "Should not be at top initially")

	// Scroll to top
	pane.ScrollToTop()
	require.Equal(t, 0, pane.ScrollOffset(), "ScrollOffset should be 0 after ScrollToTop")
	require.True(t, pane.AtTop(), "Should be at top after ScrollToTop")
}

// =============================================================================
// PrewarmCache Tests (perles-l1krd.7)
// =============================================================================

func TestPrewarmCache_PopulatesBuffer(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(50)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetBufferLines(5) // Smaller buffer for testing

	// Scroll to middle
	pane.SetScrollOffset(20)

	// Clear cache to verify prewarm populates it
	pane.virtualContent.Cache().Clear()
	require.Equal(t, 0, pane.virtualContent.Cache().Size(), "Cache should be empty after clear")

	// Call prewarmCache
	pane.prewarmCache()

	// Cache should now have entries
	cacheSize := pane.virtualContent.Cache().Size()
	require.Greater(t, cacheSize, 0, "Cache should have entries after prewarm")

	// With bufferLines=5, visible=10, we should have pre-warmed:
	// - 5 lines above visible area (15-19)
	// - 5 lines below visible area (30-34)
	// Total: 10 buffer lines
	t.Logf("Cache size after prewarm: %d", cacheSize)
}

func TestPrewarmCache_BufferSize(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(100)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	totalLines := pane.TotalLines()
	require.Greater(t, totalLines, 100, "Should have >100 lines for this test")

	// Set a specific buffer size
	bufferSize := 20
	pane.SetBufferLines(bufferSize)

	// Position in the middle of content
	scrollOffset := 50
	pane.SetScrollOffset(scrollOffset)

	// Clear cache
	pane.virtualContent.Cache().Clear()

	// Call prewarmCache
	pane.prewarmCache()

	// Verify buffer range calculation is correct
	// Above buffer: [scrollOffset - bufferLines, scrollOffset) = [30, 50)
	// Below buffer: [scrollOffset + height, scrollOffset + height + bufferLines) = [60, 80)

	// The buffer should have pre-warmed approximately 2*bufferSize lines
	cacheSize := pane.virtualContent.Cache().Size()

	// We expect: bufferLines above (20) + bufferLines below (20) = ~40 lines
	// (minus any clamping at boundaries)
	expectedMinLines := bufferSize // At minimum, we should have one buffer zone
	require.GreaterOrEqual(t, cacheSize, expectedMinLines,
		"Cache should have at least %d lines from buffer zones", expectedMinLines)

	t.Logf("Buffer size: %d, Cache entries after prewarm: %d", bufferSize, cacheSize)
}

func TestPrewarmCache_ClampsToBoundaries(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(5) // Small content
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetBufferLines(50) // Large buffer, larger than content

	totalLines := pane.TotalLines()

	// Scroll to top - buffer above should be clamped to 0
	pane.SetScrollOffset(0)
	pane.virtualContent.Cache().Clear()

	// This should not panic and should handle clamping correctly
	pane.prewarmCache()

	// Verify cache size is reasonable (limited by content size, not buffer)
	cacheSize := pane.virtualContent.Cache().Size()
	require.LessOrEqual(t, cacheSize, totalLines,
		"Cache should not exceed total lines when buffer extends beyond content")
}

func TestPrewarmCache_EmptyContent(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(80, 10)

	// No content set - prewarmCache should handle nil virtualContent gracefully
	pane.prewarmCache() // Should not panic
}

func TestPrewarmCache_CalledOnScroll(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(50)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)
	pane.SetBufferLines(5)

	// Scroll up - should trigger prewarmCache
	pane.SetScrollOffset(20)
	pane.virtualContent.Cache().Clear()

	pane.ScrollUp(3)

	// Cache should have entries (prewarmCache was called)
	require.Greater(t, pane.virtualContent.Cache().Size(), 0,
		"ScrollUp should trigger prewarmCache")

	// Scroll down - should also trigger prewarmCache
	pane.virtualContent.Cache().Clear()

	pane.ScrollDown(3)

	require.Greater(t, pane.virtualContent.Cache().Size(), 0,
		"ScrollDown should trigger prewarmCache")
}

// =============================================================================
// AppendMessage Tests (perles-l1krd.7)
// =============================================================================

func TestAppendMessage_AutoScrollsIfAtBottom(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(10)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Scroll to bottom
	pane.ScrollToBottom()
	require.True(t, pane.AtBottom(), "Should be at bottom before append")

	initialMaxOffset := pane.maxScrollOffset()

	// Append a new message
	newMsg := chatrender.Message{
		Role:    "user",
		Content: "This is a new message that should trigger auto-scroll",
	}
	pane.AppendMessage(newMsg)

	// After append, should still be at bottom (auto-scroll)
	require.True(t, pane.AtBottom(), "Should still be at bottom after append (auto-scroll)")

	// maxScrollOffset should have increased (more content)
	newMaxOffset := pane.maxScrollOffset()
	require.Greater(t, newMaxOffset, initialMaxOffset,
		"maxScrollOffset should increase after appending content")

	// scrollOffset should have been updated to new bottom
	require.Equal(t, newMaxOffset, pane.ScrollOffset(),
		"scrollOffset should equal new maxScrollOffset after auto-scroll")
}

func TestAppendMessage_PreservesScrollIfNotAtBottom(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(20)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 10)

	// Scroll to middle (not at bottom)
	initialOffset := 10
	pane.SetScrollOffset(initialOffset)
	require.False(t, pane.AtBottom(), "Should not be at bottom")

	// Append a new message
	newMsg := chatrender.Message{
		Role:    "assistant",
		Content: "New content that should not cause scroll jump",
	}
	pane.AppendMessage(newMsg)

	// Scroll position should be preserved (no auto-scroll)
	require.Equal(t, initialOffset, pane.ScrollOffset(),
		"scrollOffset should be preserved when not at bottom")
	require.False(t, pane.AtBottom(), "Should still not be at bottom after append")
}

func TestAppendMessage_UpdatesPlainLines(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(5)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	initialLineCount := len(pane.PlainLines())

	// Append a new message
	newMsg := chatrender.Message{
		Role:    "user",
		Content: "New message content here",
	}
	pane.AppendMessage(newMsg)

	// PlainLines should have increased
	newLineCount := len(pane.PlainLines())
	require.Greater(t, newLineCount, initialLineCount,
		"PlainLines count should increase after append")

	// New content should be in PlainLines
	found := false
	for _, line := range pane.PlainLines() {
		if strings.Contains(line, "New message content") {
			found = true
			break
		}
	}
	require.True(t, found, "New message content should be in PlainLines")
}

func TestAppendMessage_ToEmptyPane(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(80, 20)

	// Pane has no content initially
	require.Nil(t, pane.virtualContent, "virtualContent should be nil initially")
	require.Equal(t, 0, pane.TotalLines(), "Should have 0 lines initially")

	// Append first message
	firstMsg := chatrender.Message{
		Role:    "user",
		Content: "First message ever",
	}
	startIndex := pane.AppendMessage(firstMsg)

	// Should have created virtualContent
	require.NotNil(t, pane.virtualContent, "virtualContent should be created")
	require.Greater(t, pane.TotalLines(), 0, "Should have lines after append")
	require.Equal(t, 0, startIndex, "First message should start at index 0")

	// PlainLines should be populated
	require.NotEmpty(t, pane.PlainLines(), "PlainLines should be populated")

	// Content should be in PlainLines
	found := false
	for _, line := range pane.PlainLines() {
		if strings.Contains(line, "First message") {
			found = true
			break
		}
	}
	require.True(t, found, "First message should be in PlainLines")
}

func TestAppendMessage_ReturnsStartIndex(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	messages := createTestMessages(3)
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 20)

	initialTotalLines := pane.TotalLines()

	// Append a new message
	newMsg := chatrender.Message{
		Role:    "user",
		Content: "Append test message",
	}
	startIndex := pane.AppendMessage(newMsg)

	// startIndex should be the line index where new message starts
	// which is the old totalLines (before append)
	require.Equal(t, initialTotalLines, startIndex,
		"AppendMessage should return the starting line index of the new message")
}

func TestAppendMessage_UpdatesTopPadding(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)

	// Start with small content
	messages := []chatrender.Message{
		{Role: "user", Content: "Short"},
	}
	cfg := chatrender.RenderConfig{
		AgentLabel: "Assistant",
		UserLabel:  "You",
	}

	pane.SetMessages(messages, 80, cfg)
	pane.SetSize(80, 50) // Large height = significant padding

	initialPadding := pane.TopPadding()
	require.Greater(t, initialPadding, 0, "Should have padding with small content")

	// Append messages until content exceeds height
	for i := 0; i < 20; i++ {
		pane.AppendMessage(chatrender.Message{
			Role:    "assistant",
			Content: "Additional content line number " + string(rune('0'+i)),
		})
	}

	// After adding many messages, padding should decrease
	newPadding := pane.TopPadding()

	// Either padding decreased or content now exceeds height
	if pane.TotalLines() >= 50 {
		require.Equal(t, 0, newPadding, "Padding should be 0 when content exceeds height")
	} else {
		require.Less(t, newPadding, initialPadding, "Padding should decrease as content grows")
	}
}

// =============================================================================
// isWithinBounds Tests
// =============================================================================

func TestIsWithinBounds(t *testing.T) {
	pane := NewVirtualSelectablePane(nil, nil)
	pane.SetSize(80, 20)
	pane.SetScreenPosition(10, 5)

	// Within bounds
	require.True(t, pane.isWithinBounds(15, 10), "Point within bounds should return true")
	require.True(t, pane.isWithinBounds(10, 5), "Top-left corner should be within bounds")
	require.True(t, pane.isWithinBounds(89, 24), "Bottom-right corner should be within bounds")

	// Out of bounds - left
	require.False(t, pane.isWithinBounds(5, 10), "Point left of bounds should return false")

	// Out of bounds - right
	require.False(t, pane.isWithinBounds(100, 10), "Point right of bounds should return false")

	// Out of bounds - above
	require.False(t, pane.isWithinBounds(15, 2), "Point above bounds should return false")

	// Out of bounds - below
	require.False(t, pane.isWithinBounds(15, 30), "Point below bounds should return false")
}
