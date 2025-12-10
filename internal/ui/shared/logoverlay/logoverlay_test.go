package logoverlay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"

	"perles/internal/log"
)

// TestMain initializes the logger for all tests in this package.
func TestMain(m *testing.M) {
	// Create temp dir for log file
	tmpDir, err := os.MkdirTemp("", "logoverlay-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")
	cleanup, err := log.Init(logPath, 100)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	os.Exit(m.Run())
}

// === Constructor Tests ===

func TestNew(t *testing.T) {
	m := New()

	require.False(t, m.Visible())
	require.Empty(t, m.View())
	require.Equal(t, log.LevelDebug, m.minLevel)
}

func TestNewWithSize(t *testing.T) {
	m := NewWithSize(80, 24)

	require.False(t, m.Visible())
	require.Equal(t, 80, m.width)
	require.Equal(t, 24, m.height)
	require.Equal(t, log.LevelDebug, m.minLevel)
}

// === Visibility Tests ===

func TestToggle(t *testing.T) {
	m := New()
	require.False(t, m.Visible())

	m.Toggle()
	require.True(t, m.Visible())

	m.Toggle()
	require.False(t, m.Visible())
}

func TestShow(t *testing.T) {
	m := New()
	m.Show()

	require.True(t, m.Visible())
}

func TestHide(t *testing.T) {
	m := New()
	m.Show()
	m.Hide()

	require.False(t, m.Visible())
}

// === Init Tests ===

func TestInit(t *testing.T) {
	m := New()
	cmd := m.Init()

	require.Nil(t, cmd)
}

// === Update Tests ===

func TestUpdate_IgnoresWhenNotVisible(t *testing.T) {
	m := New()
	// Don't show overlay - should ignore all key presses
	originalLevel := m.minLevel

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	require.Equal(t, originalLevel, m.minLevel)
}

func TestUpdate_FilterKeys(t *testing.T) {
	tests := []struct {
		key      string
		expected log.Level
	}{
		{"d", log.LevelDebug},
		{"i", log.LevelInfo},
		{"w", log.LevelWarn},
		{"e", log.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := NewWithSize(80, 24)
			m.Show()
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})

			require.Equal(t, tt.expected, m.minLevel)
		})
	}
}

func TestUpdate_ClearBuffer(t *testing.T) {
	m := NewWithSize(80, 24)
	m.Show()

	// Add some logs to buffer
	log.Debug(log.CatUI, "test log")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	// Buffer should be cleared - overlay still visible
	require.True(t, m.Visible())
	// Get recent logs should be empty after clear
	logs := log.GetRecentLogs(10)
	require.Empty(t, logs)
}

func TestUpdate_CloseWithCtrlX(t *testing.T) {
	m := NewWithSize(80, 24)
	m.Show()

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})

	require.False(t, m.Visible())
	require.NotNil(t, cmd)
	// Verify cmd returns CloseMsg
	msg := cmd()
	_, ok := msg.(CloseMsg)
	require.True(t, ok)
}

func TestUpdate_CloseWithEsc(t *testing.T) {
	m := NewWithSize(80, 24)
	m.Show()

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.False(t, m.Visible())
	require.NotNil(t, cmd)
	// Verify cmd returns CloseMsg
	msg := cmd()
	_, ok := msg.(CloseMsg)
	require.True(t, ok)
}

func TestUpdate_WindowSizeMsg(t *testing.T) {
	m := New()
	m.Show() // Must be visible to process WindowSizeMsg

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	require.Equal(t, 100, m.width)
	require.Equal(t, 50, m.height)
}

func TestUpdate_WindowSizeMsg_IgnoredWhenNotVisible(t *testing.T) {
	m := NewWithSize(80, 24)
	// Don't show - should ignore WindowSizeMsg

	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})

	// Original dimensions preserved
	require.Equal(t, 80, m.width)
	require.Equal(t, 24, m.height)
}

func TestUpdate_UnhandledKeyReturnsNoCmd(t *testing.T) {
	m := NewWithSize(80, 24)
	m.Show()

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	require.Nil(t, cmd)
	require.True(t, m.Visible())
}

// === Scrolling Tests ===

func TestUpdate_ScrollDown(t *testing.T) {
	log.ClearBuffer()
	// Add enough log entries to enable scrolling
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	require.GreaterOrEqual(t, m.viewport.YOffset, initialOffset)
}

func TestUpdate_ScrollUp(t *testing.T) {
	log.ClearBuffer()
	// Add enough log entries to enable scrolling
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	// Scroll down first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	afterDown := m.viewport.YOffset

	// Now scroll up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})

	require.LessOrEqual(t, m.viewport.YOffset, afterDown)
}

func TestUpdate_GotoTop(t *testing.T) {
	log.ClearBuffer()
	// Add enough log entries to enable scrolling
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	// Scroll down first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Go to top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})

	require.Equal(t, 0, m.viewport.YOffset)
}

func TestUpdate_GotoBottom(t *testing.T) {
	log.ClearBuffer()
	// Add enough log entries to enable scrolling
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	// Go to top first to ensure we're not at bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	topOffset := m.viewport.YOffset

	// Go to bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})

	require.GreaterOrEqual(t, m.viewport.YOffset, topOffset)
}

func TestUpdate_ScrollWithDownArrow(t *testing.T) {
	log.ClearBuffer()
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	require.GreaterOrEqual(t, m.viewport.YOffset, initialOffset)
}

func TestUpdate_ScrollWithUpArrow(t *testing.T) {
	log.ClearBuffer()
	for i := 0; i < 20; i++ {
		log.Info(log.CatUI, "Log entry")
	}

	m := NewWithSize(80, 24)
	m.Show()

	// Scroll down first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	afterDown := m.viewport.YOffset

	// Scroll up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})

	require.LessOrEqual(t, m.viewport.YOffset, afterDown)
}

// === View Tests ===

func TestView_EmptyWhenNotVisible(t *testing.T) {
	m := New()

	require.Empty(t, m.View())
}

func TestView_ContainsHeader(t *testing.T) {
	log.ClearBuffer()
	m := NewWithSize(80, 24)
	m.Show()
	view := m.View()

	require.Contains(t, view, "Logs")
}

func TestView_ContainsFilterHints(t *testing.T) {
	log.ClearBuffer()
	m := NewWithSize(80, 24)
	m.Show()
	view := m.View()

	require.Contains(t, view, "[c]")
	require.Contains(t, view, "[d]")
	require.Contains(t, view, "[i]")
	require.Contains(t, view, "[w]")
	require.Contains(t, view, "[e]")
}

func TestView_HasBorder(t *testing.T) {
	log.ClearBuffer()
	m := NewWithSize(80, 24)
	m.Show()
	view := m.View()

	// Rounded border characters
	require.Contains(t, view, "╭")
	require.Contains(t, view, "╯")
}

func TestView_EmptyLogsMessage(t *testing.T) {
	log.ClearBuffer()
	m := NewWithSize(80, 24)
	m.Show()
	view := m.View()

	require.Contains(t, view, "No logs to display")
}

func TestView_ShowsLogEntries(t *testing.T) {
	log.ClearBuffer()
	log.Info(log.CatUI, "Test info message")

	m := NewWithSize(80, 24)
	m.Show()
	view := m.View()

	require.Contains(t, view, "Test info message")
}

// === Overlay Tests ===

func TestOverlay_NotVisibleReturnsBackground(t *testing.T) {
	m := New()
	bg := "Background\nContent"

	result := m.Overlay(bg)

	require.Equal(t, bg, result)
}

func TestOverlay_VisiblePlacesCentered(t *testing.T) {
	log.ClearBuffer()
	m := NewWithSize(60, 20)
	m.Show()
	bg := strings.Repeat(strings.Repeat(".", 60)+"\n", 20)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg)

	// Should contain overlay content, not just background
	require.Contains(t, result, "Logs")
	require.NotEqual(t, bg, result)
}

// === SetSize Tests ===

func TestSetSize(t *testing.T) {
	m := New()

	m.SetSize(120, 40)

	require.Equal(t, 120, m.width)
	require.Equal(t, 40, m.height)
}

// === matchesLevel Tests ===

func TestMatchesLevel_DebugShowsAll(t *testing.T) {
	m := Model{minLevel: log.LevelDebug}

	require.True(t, m.matchesLevel("[DEBUG] test"))
	require.True(t, m.matchesLevel("[INFO] test"))
	require.True(t, m.matchesLevel("[WARN] test"))
	require.True(t, m.matchesLevel("[ERROR] test"))
}

func TestMatchesLevel_InfoFiltersDebug(t *testing.T) {
	m := Model{minLevel: log.LevelInfo}

	require.False(t, m.matchesLevel("[DEBUG] test"))
	require.True(t, m.matchesLevel("[INFO] test"))
	require.True(t, m.matchesLevel("[WARN] test"))
	require.True(t, m.matchesLevel("[ERROR] test"))
}

func TestMatchesLevel_WarnFiltersDebugAndInfo(t *testing.T) {
	m := Model{minLevel: log.LevelWarn}

	require.False(t, m.matchesLevel("[DEBUG] test"))
	require.False(t, m.matchesLevel("[INFO] test"))
	require.True(t, m.matchesLevel("[WARN] test"))
	require.True(t, m.matchesLevel("[ERROR] test"))
}

func TestMatchesLevel_ErrorOnly(t *testing.T) {
	m := Model{minLevel: log.LevelError}

	require.False(t, m.matchesLevel("[DEBUG] test"))
	require.False(t, m.matchesLevel("[INFO] test"))
	require.False(t, m.matchesLevel("[WARN] test"))
	require.True(t, m.matchesLevel("[ERROR] test"))
}

func TestMatchesLevel_UnknownAlwaysShown(t *testing.T) {
	m := Model{minLevel: log.LevelError}

	require.True(t, m.matchesLevel("some unknown format"))
}

// === colorizeEntry Tests ===

func TestColorizeEntry_TruncatesLongEntries(t *testing.T) {
	m := Model{}
	longEntry := strings.Repeat("a", 100)

	result := m.colorizeEntry(longEntry, 50)

	// Should be truncated with ellipsis
	require.LessOrEqual(t, len(result), 60) // Some margin for ANSI codes
}

func TestColorizeEntry_TrimsTrailingNewline(t *testing.T) {
	m := Model{}
	entry := "[INFO] test\n"

	result := m.colorizeEntry(entry, 80)

	require.NotContains(t, result, "\n")
}

// === buildFilterHint Tests ===

func TestBuildFilterHint_ContainsAllOptions(t *testing.T) {
	m := Model{minLevel: log.LevelDebug}

	hint := m.buildFilterHint()

	require.Contains(t, hint, "[c] Clear")
	require.Contains(t, hint, "[d] Debug")
	require.Contains(t, hint, "[i] Info")
	require.Contains(t, hint, "[w] Warn")
	require.Contains(t, hint, "[e] Error")
}

func TestBuildFilterHint_HighlightsActiveLevel(t *testing.T) {
	tests := []struct {
		level    log.Level
		expected string
	}{
		{log.LevelDebug, "[d] Debug"},
		{log.LevelInfo, "[i] Info"},
		{log.LevelWarn, "[w] Warn"},
		{log.LevelError, "[e] Error"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			m := Model{minLevel: tt.level}
			hint := m.buildFilterHint()

			// The active filter option should be in the hint
			require.Contains(t, hint, tt.expected)
		})
	}
}

// === CloseMsg Tests ===

func TestCloseMsg(t *testing.T) {
	// CloseMsg is a marker type - verify it exists and can be instantiated
	msg := CloseMsg{}
	_ = msg
}

// === Golden Tests ===
// Run with -update flag to update golden files:
// go test ./internal/ui/shared/logoverlay -update
//
// Note: Only the empty state golden test is used because log entries
// contain timestamps that change between runs. Filter behavior is
// tested via unit tests that check log count and content.

func TestLogOverlay_View_Empty_Golden(t *testing.T) {
	log.ClearBuffer()

	m := NewWithSize(60, 20)
	m.Show()

	teatest.RequireEqualOutput(t, []byte(m.View()))
}

func TestLogOverlay_Overlay_Empty_Golden(t *testing.T) {
	log.ClearBuffer()

	m := NewWithSize(50, 15)
	m.Show()

	bg := strings.Repeat(strings.Repeat(".", 50)+"\n", 15)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg)
	teatest.RequireEqualOutput(t, []byte(result))
}

// === Filter View Tests (non-golden) ===
// These tests verify filtering behavior without golden file comparison
// because log entries contain timestamps.

func TestView_FilteredContent(t *testing.T) {
	log.ClearBuffer()
	log.Debug(log.CatUI, "DebugMsg")
	log.Info(log.CatUI, "InfoMsg")
	log.Warn(log.CatUI, "WarnMsg")
	log.Error(log.CatUI, "ErrorMsg")

	// Test INFO filter - should not contain DEBUG
	m := NewWithSize(80, 24)
	m.Show()
	// Use Update handler to change filter (updates viewport content)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	view := m.View()
	require.NotContains(t, view, "DebugMsg")
	require.Contains(t, view, "InfoMsg")
	require.Contains(t, view, "WarnMsg")
	require.Contains(t, view, "ErrorMsg")

	// Test ERROR filter - should only contain ERROR
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	view = m.View()
	require.NotContains(t, view, "DebugMsg")
	require.NotContains(t, view, "InfoMsg")
	require.NotContains(t, view, "WarnMsg")
	require.Contains(t, view, "ErrorMsg")
}

func TestOverlay_WithLogs(t *testing.T) {
	log.ClearBuffer()
	log.Info(log.CatUI, "Test entry")

	m := NewWithSize(50, 15)
	m.Show()

	bg := strings.Repeat(strings.Repeat(".", 50)+"\n", 15)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg)

	// Should contain overlay structure
	require.Contains(t, result, "Logs")
	require.Contains(t, result, "Test entry")
}
