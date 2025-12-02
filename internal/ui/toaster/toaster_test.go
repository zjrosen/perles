package toaster

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	m := New()

	assert.False(t, m.Visible())
	assert.Empty(t, m.View())
}

func TestShow(t *testing.T) {
	m := New().Show("Hello", StyleSuccess)

	assert.True(t, m.Visible())
	assert.Contains(t, m.View(), "Hello")
}

func TestHide(t *testing.T) {
	m := New().Show("Hello", StyleSuccess).Hide()

	assert.False(t, m.Visible())
	assert.Empty(t, m.View())
}

func TestShow_ReplacesExisting(t *testing.T) {
	m := New().
		Show("First", StyleSuccess).
		Show("Second", StyleError)

	assert.True(t, m.Visible())
	assert.Contains(t, m.View(), "Second")
	assert.NotContains(t, m.View(), "First")
}

func TestView_EmptyWhenNotVisible(t *testing.T) {
	m := New()

	assert.Empty(t, m.View())
}

func TestView_EmptyWhenMessageEmpty(t *testing.T) {
	m := Model{visible: true, message: ""}

	assert.Empty(t, m.View())
}

func TestView_StyleSuccess(t *testing.T) {
	m := New().Show("Success!", StyleSuccess)
	view := m.View()

	// Should contain the message with ✅ emoji and have a border
	assert.Contains(t, view, "✅")
	assert.Contains(t, view, "Success!")
	assert.Contains(t, view, "╭") // Rounded border corner
}

func TestView_StyleError(t *testing.T) {
	m := New().Show("Error!", StyleError)
	view := m.View()

	// Should contain the message with ❌ emoji
	assert.Contains(t, view, "❌")
	assert.Contains(t, view, "Error!")
	assert.Contains(t, view, "╭")
}

func TestView_StyleInfo(t *testing.T) {
	m := New().Show("Switched view", StyleInfo)
	view := m.View()

	// Should contain the message with ℹ️ emoji
	assert.Contains(t, view, "ℹ️")
	assert.Contains(t, view, "Switched view")
	assert.Contains(t, view, "╭")
}

func TestView_StyleWarn(t *testing.T) {
	m := New().Show("Caution!", StyleWarn)
	view := m.View()

	// Should contain the message with ⚠️ emoji
	assert.Contains(t, view, "⚠️")
	assert.Contains(t, view, "Caution!")
	assert.Contains(t, view, "╭")
}

func TestSetSize(t *testing.T) {
	m := New().SetSize(80, 24)

	assert.Equal(t, 80, m.width)
	assert.Equal(t, 24, m.height)
}

func TestOverlay_NotVisibleReturnsBackground(t *testing.T) {
	m := New()
	bg := "Background\nContent"

	result := m.Overlay(bg, 20, 10)

	assert.Equal(t, bg, result)
}

func TestOverlay_VisiblePlacesAtBottom(t *testing.T) {
	m := New().Show("Toast", StyleSuccess)
	// Create background with dots
	bg := strings.Repeat(strings.Repeat(".", 20)+"\n", 10)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg, 20, 10)

	lines := strings.Split(result, "\n")
	// Toast should be near the bottom (with padding)
	bottomLines := lines[len(lines)-5:]
	found := false
	for _, line := range bottomLines {
		if strings.Contains(line, "Toast") {
			found = true
			break
		}
	}
	assert.True(t, found, "Toast should appear near the bottom of the overlay")
}

func TestOverlay_EmptyMessageReturnsBackground(t *testing.T) {
	m := Model{visible: true, message: ""}
	bg := "Background"

	result := m.Overlay(bg, 20, 10)

	assert.Equal(t, bg, result)
}

func TestDismissMsg(t *testing.T) {
	// DismissMsg is just a marker type, verify it exists
	msg := DismissMsg{}
	_ = msg
}

func TestScheduleDismiss(t *testing.T) {
	// ScheduleDismiss returns a tea.Cmd, verify it's not nil
	cmd := ScheduleDismiss(0)
	assert.NotNil(t, cmd)
}

func TestVisible_ImmutableModel(t *testing.T) {
	m1 := New()
	m2 := m1.Show("Hello", StyleSuccess)

	// Original should be unchanged
	assert.False(t, m1.Visible())
	assert.True(t, m2.Visible())
}

func TestHide_ImmutableModel(t *testing.T) {
	m1 := New().Show("Hello", StyleSuccess)
	m2 := m1.Hide()

	// Original should be unchanged
	assert.True(t, m1.Visible())
	assert.False(t, m2.Visible())
}

// TestOverlay_Success_Golden tests success toast overlay rendering.
// Run with -update flag to update golden files: go test ./internal/ui/toaster -update
func TestOverlay_Success_Golden(t *testing.T) {
	m := New().Show("synced issues", StyleSuccess)
	bg := strings.Repeat(strings.Repeat(".", 30)+"\n", 12)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg, 30, 12)
	teatest.RequireEqualOutput(t, []byte(result))
}

// TestOverlay_Error_Golden tests error toast overlay rendering.
func TestOverlay_Error_Golden(t *testing.T) {
	m := New().Show("failed to save", StyleError)
	bg := strings.Repeat(strings.Repeat(".", 30)+"\n", 12)
	bg = strings.TrimSuffix(bg, "\n")

	result := m.Overlay(bg, 30, 12)
	teatest.RequireEqualOutput(t, []byte(result))
}

// TestView_Success_Golden tests the success style toast box rendering.
func TestView_Success_Golden(t *testing.T) {
	m := New().Show("Column saved", StyleSuccess)
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

// TestView_Error_Golden tests the error style toast box rendering.
func TestView_Error_Golden(t *testing.T) {
	m := New().Show("failed to save", StyleError)
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

// TestView_Info_Golden tests the info style toast box rendering.
func TestView_Info_Golden(t *testing.T) {
	m := New().Show("Switched to My Tasks", StyleInfo)
	teatest.RequireEqualOutput(t, []byte(m.View()))
}

// TestView_Warn_Golden tests the warn style toast box rendering.
func TestView_Warn_Golden(t *testing.T) {
	m := New().Show("Unsaved changes", StyleWarn)
	teatest.RequireEqualOutput(t, []byte(m.View()))
}
