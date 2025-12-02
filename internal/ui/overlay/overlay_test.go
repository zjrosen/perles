package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
)

func TestPlace_Center(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA"
	fg := "XX\nXX"
	cfg := Config{Width: 5, Height: 3, Position: Center}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 3)
	// Middle line should have XX centered (position 1-2 in 0-4)
	assert.Contains(t, lines[1], "XX")
}

func TestPlace_Center_LargeForeground(t *testing.T) {
	bg := "AAA\nAAA\nAAA"
	fg := "XXXXX\nXXXXX"
	cfg := Config{Width: 3, Height: 3, Position: Center}

	result := Place(cfg, fg, bg)

	// Should not panic, fg is placed starting at x=0, y=0
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 3)
	// Foreground overwrites background starting from position 0
	assert.True(t, strings.HasPrefix(lines[0], "XXXXX") || strings.HasPrefix(lines[1], "XXXXX"))
}

func TestPlace_Top(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA\nAAAAA\nAAAAA"
	fg := "XX"
	cfg := Config{Width: 5, Height: 5, Position: Top, PadY: 0}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	// First line should contain XX (centered horizontally)
	assert.Contains(t, lines[0], "XX")
	// Last line should still be background
	assert.Equal(t, "AAAAA", lines[4])
}

func TestPlace_Top_WithPadding(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA\nAAAAA\nAAAAA"
	fg := "XX"
	cfg := Config{Width: 5, Height: 5, Position: Top, PadY: 1}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	// First line should be untouched background
	assert.Equal(t, "AAAAA", lines[0])
	// Second line should contain XX
	assert.Contains(t, lines[1], "XX")
}

func TestPlace_Bottom(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA\nAAAAA\nAAAAA"
	fg := "XX"
	cfg := Config{Width: 5, Height: 5, Position: Bottom, PadY: 0}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	// Last line should contain XX
	assert.Contains(t, lines[4], "XX")
	// First line should still be background
	assert.Equal(t, "AAAAA", lines[0])
}

func TestPlace_Bottom_WithPadding(t *testing.T) {
	bg := "AAAAA\nAAAAA\nAAAAA\nAAAAA\nAAAAA"
	fg := "XX"
	cfg := Config{Width: 5, Height: 5, Position: Bottom, PadY: 1}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	// Last line should be untouched background
	assert.Equal(t, "AAAAA", lines[4])
	// Second to last should contain XX
	assert.Contains(t, lines[3], "XX")
}

func TestPlace_EmptyBackground(t *testing.T) {
	bg := ""
	fg := "XX\nXX"
	cfg := Config{Width: 5, Height: 3, Position: Center}

	result := Place(cfg, fg, bg)

	// Should pad background and place foreground
	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 3)
}

func TestPlace_PreservesBackgroundOnSides(t *testing.T) {
	bg := "ABCDE\nFGHIJ\nKLMNO"
	fg := "X"
	cfg := Config{Width: 5, Height: 3, Position: Center}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	// Middle line should have X in center with F and J preserved
	// X is at position 2, so we expect FG on left, IJ on right
	assert.Equal(t, "FGXIJ", lines[1])
}

func TestPlace_PreservesANSI(t *testing.T) {
	// Red colored background
	bg := "\x1b[31mRED\x1b[0m\n\x1b[31mRED\x1b[0m\n\x1b[31mRED\x1b[0m"
	fg := "X"
	cfg := Config{Width: 3, Height: 3, Position: Center}

	result := Place(cfg, fg, bg)

	// Result should still contain ANSI codes
	assert.Contains(t, result, "\x1b[31m")
}

func TestPlace_MultilineForeground(t *testing.T) {
	bg := ".....\n.....\n.....\n.....\n....."
	fg := "XXX\nXXX\nXXX"
	cfg := Config{Width: 5, Height: 5, Position: Center}

	result := Place(cfg, fg, bg)

	lines := strings.Split(result, "\n")
	assert.Len(t, lines, 5)
	// Lines 1, 2, 3 should contain XXX (centered at position 1)
	assert.Contains(t, lines[1], "XXX")
	assert.Contains(t, lines[2], "XXX")
	assert.Contains(t, lines[3], "XXX")
}

func TestCalculatePosition_Center(t *testing.T) {
	cfg := Config{Width: 10, Height: 10, Position: Center}

	x, y := calculatePosition(cfg, 4, 2)

	assert.Equal(t, 3, x) // (10-4)/2 = 3
	assert.Equal(t, 4, y) // (10-2)/2 = 4
}

func TestCalculatePosition_Top(t *testing.T) {
	cfg := Config{Width: 10, Height: 10, Position: Top, PadY: 2}

	x, y := calculatePosition(cfg, 4, 2)

	assert.Equal(t, 3, x) // (10-4)/2 = 3
	assert.Equal(t, 2, y) // PadY = 2
}

func TestCalculatePosition_Bottom(t *testing.T) {
	cfg := Config{Width: 10, Height: 10, Position: Bottom, PadY: 1}

	x, y := calculatePosition(cfg, 4, 2)

	assert.Equal(t, 3, x) // (10-4)/2 = 3
	assert.Equal(t, 7, y) // 10 - 2 - 1 = 7
}

func TestCalculatePosition_NegativeClamping(t *testing.T) {
	// Foreground larger than viewport
	cfg := Config{Width: 5, Height: 5, Position: Center}

	x, y := calculatePosition(cfg, 10, 10)

	// Should clamp to 0, not negative
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)
}

// TestPlace_Center_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test perles/internal/ui/overlay -update
func TestPlace_Center_Golden(t *testing.T) {
	bg := strings.Repeat(strings.Repeat(".", 20)+"\n", 10)
	bg = strings.TrimSuffix(bg, "\n")
	fg := "┌──────┐\n│ HELP │\n└──────┘"
	cfg := Config{Width: 20, Height: 10, Position: Center}

	result := Place(cfg, fg, bg)
	teatest.RequireEqualOutput(t, []byte(result))
}

// TestPlace_Top_Golden tests top positioning with padding
func TestPlace_Top_Golden(t *testing.T) {
	bg := strings.Repeat(strings.Repeat(".", 20)+"\n", 10)
	bg = strings.TrimSuffix(bg, "\n")
	fg := "[ STATUS BAR ]"
	cfg := Config{Width: 20, Height: 10, Position: Top, PadY: 1}

	result := Place(cfg, fg, bg)
	teatest.RequireEqualOutput(t, []byte(result))
}

// TestPlace_Bottom_Golden tests bottom positioning with padding
func TestPlace_Bottom_Golden(t *testing.T) {
	bg := strings.Repeat(strings.Repeat(".", 20)+"\n", 10)
	bg = strings.TrimSuffix(bg, "\n")
	fg := "[ FOOTER ]"
	cfg := Config{Width: 20, Height: 10, Position: Bottom, PadY: 1}

	result := Place(cfg, fg, bg)
	teatest.RequireEqualOutput(t, []byte(result))
}
