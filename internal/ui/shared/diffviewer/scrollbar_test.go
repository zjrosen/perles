package diffviewer

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// ===========================================================================
// Tests for ScrollbarConfig types
// ===========================================================================

func TestScrollbarConfig_ZeroValue(t *testing.T) {
	var cfg ScrollbarConfig
	require.Equal(t, 0, cfg.TotalLines, "zero TotalLines should be 0")
	require.Equal(t, 0, cfg.ViewportHeight, "zero ViewportHeight should be 0")
	require.Equal(t, 0, cfg.ScrollOffset, "zero ScrollOffset should be 0")
	require.Equal(t, "", cfg.TrackChar, "zero TrackChar should be empty")
	require.Equal(t, "", cfg.ThumbChar, "zero ThumbChar should be empty")
}

func TestDefaultScrollbarConfig(t *testing.T) {
	cfg := DefaultScrollbarConfig()

	require.Equal(t, "░", cfg.TrackChar, "default TrackChar should be ░")
	require.Equal(t, "█", cfg.ThumbChar, "default ThumbChar should be █")
}

// ===========================================================================
// Tests for calculateThumbBounds (5 cases)
// ===========================================================================

func TestCalculateThumbBounds_SmallFile(t *testing.T) {
	// Case 1: Small file (50 lines, 30 viewport) - thumb should be large
	cfg := ScrollbarConfig{
		TotalLines:     50,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)

	// thumbHeight = max(1, 30*30/50) = max(1, 18) = 18
	require.Equal(t, 18, height, "thumb height for small file should be 18")
	require.Equal(t, 0, start, "thumb start at offset 0 should be 0")
}

func TestCalculateThumbBounds_LargeFile(t *testing.T) {
	// Case 2: Large file (1000 lines, 30 viewport) - thumb should be small
	cfg := ScrollbarConfig{
		TotalLines:     1000,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)

	// thumbHeight = max(1, 30*30/1000) = max(1, 0.9) = 1
	require.Equal(t, 1, height, "thumb height for large file should be minimum 1")
	require.Equal(t, 0, start, "thumb start at offset 0 should be 0")
}

func TestCalculateThumbBounds_ContentFitsViewport(t *testing.T) {
	// Case 3: Edge case - totalLines == viewportHeight - thumb fills viewport
	cfg := ScrollbarConfig{
		TotalLines:     30,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)

	require.Equal(t, 30, height, "thumb should fill entire viewport when content fits")
	require.Equal(t, 0, start, "thumb start should be 0 when content fits")

	// Also test when totalLines < viewportHeight
	cfg.TotalLines = 20
	start, height = calculateThumbBounds(cfg)
	require.Equal(t, 30, height, "thumb should fill viewport when content is smaller")
	require.Equal(t, 0, start, "thumb start should be 0 when content is smaller")
}

func TestCalculateThumbBounds_ZeroTotalLines(t *testing.T) {
	// Case 4: Edge case - totalLines == 0 - handle gracefully
	cfg := ScrollbarConfig{
		TotalLines:     0,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)

	require.Equal(t, 0, height, "thumb height should be 0 when no content")
	require.Equal(t, 0, start, "thumb start should be 0 when no content")
}

func TestCalculateThumbBounds_ZeroViewportHeight(t *testing.T) {
	// Additional edge case: viewport height is 0
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 0,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)

	require.Equal(t, 0, height, "thumb height should be 0 when viewport is 0")
	require.Equal(t, 0, start, "thumb start should be 0 when viewport is 0")
}

func TestCalculateThumbBounds_ScrollAtEnd(t *testing.T) {
	// Case 5: Scroll position at end - thumb at bottom
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 30,
		ScrollOffset:   70, // maxOffset = 100 - 30 = 70
	}

	start, height := calculateThumbBounds(cfg)

	// thumbHeight = max(1, 30*30/100) = 9
	require.Equal(t, 9, height, "thumb height should be 9")
	// Thumb should be at bottom: start = 30 - 9 = 21
	require.Equal(t, 21, start, "thumb should be at bottom when scrolled to end")
}

func TestCalculateThumbBounds_ScrollMiddle(t *testing.T) {
	// Additional case: scroll to middle
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 30,
		ScrollOffset:   35, // Middle-ish
	}

	start, height := calculateThumbBounds(cfg)

	// thumbHeight = max(1, 30*30/100) = 9
	require.Equal(t, 9, height, "thumb height should be 9")
	// start should be roughly in the middle of the scrollable area
	require.True(t, start > 0 && start < 21, "thumb should be in the middle area, got %d", start)
}

func TestCalculateThumbBounds_NegativeValues(t *testing.T) {
	// Edge case: negative values should be handled gracefully
	cfg := ScrollbarConfig{
		TotalLines:     -10,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}

	start, height := calculateThumbBounds(cfg)
	require.Equal(t, 0, height, "negative totalLines should return 0 height")
	require.Equal(t, 0, start, "negative totalLines should return 0 start")

	cfg = ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: -30,
		ScrollOffset:   0,
	}

	start, height = calculateThumbBounds(cfg)
	require.Equal(t, 0, height, "negative viewport should return 0 height")
	require.Equal(t, 0, start, "negative viewport should return 0 start")
}

// ===========================================================================
// Tests for RenderScrollbar
// ===========================================================================

func TestRenderScrollbar_InvalidConfig(t *testing.T) {
	// Invalid: viewport <= 0
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 0,
		ScrollOffset:   0,
	}
	require.Empty(t, RenderScrollbar(cfg))

	// Invalid: totalLines <= 0
	cfg = ScrollbarConfig{
		TotalLines:     0,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}
	require.Empty(t, RenderScrollbar(cfg))
}

func TestRenderScrollbar_ContentFitsViewport(t *testing.T) {
	// Content fits - returns spaces
	cfg := ScrollbarConfig{
		TotalLines:     20,
		ViewportHeight: 30,
		ScrollOffset:   0,
	}
	result := RenderScrollbar(cfg)

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 30)
	for _, line := range lines {
		require.Equal(t, " ", line)
	}
}

func TestRenderScrollbar_ContentExceedsViewport(t *testing.T) {
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 30,
		ScrollOffset:   0,
		TrackChar:      "░",
		ThumbChar:      "█",
	}
	result := RenderScrollbar(cfg)

	lines := strings.Split(result, "\n")
	require.Len(t, lines, 30)
	require.NotEmpty(t, result)
}

// ===========================================================================
// Property-Based Tests (using pgregory.net/rapid)
// ===========================================================================

func TestProperty_ThumbAlwaysWithinBounds(t *testing.T) {
	// Property 1: Thumb always within bounds: 0 <= start < viewportHeight
	rapid.Check(t, func(rt *rapid.T) {
		totalLines := rapid.IntRange(0, 10000).Draw(rt, "totalLines")
		viewportHeight := rapid.IntRange(0, 100).Draw(rt, "viewportHeight")
		scrollOffset := rapid.IntRange(0, max(0, totalLines-viewportHeight)).Draw(rt, "scrollOffset")

		cfg := ScrollbarConfig{
			TotalLines:     totalLines,
			ViewportHeight: viewportHeight,
			ScrollOffset:   scrollOffset,
		}

		start, height := calculateThumbBounds(cfg)

		// For invalid configs, both should be 0
		if totalLines <= 0 || viewportHeight <= 0 {
			require.Equal(t, 0, start, "invalid config should return start=0")
			require.Equal(t, 0, height, "invalid config should return height=0")
			return
		}

		// For valid configs, start should be within bounds
		require.GreaterOrEqual(t, start, 0, "start should be >= 0")
		require.Less(t, start, viewportHeight, "start should be < viewportHeight")
	})
}

func TestProperty_ThumbHeightAlwaysAtLeastOne(t *testing.T) {
	// Property 2: Thumb height always >= 1 (when content exceeds viewport)
	rapid.Check(t, func(rt *rapid.T) {
		totalLines := rapid.IntRange(1, 10000).Draw(rt, "totalLines")
		viewportHeight := rapid.IntRange(1, 100).Draw(rt, "viewportHeight")
		scrollOffset := rapid.IntRange(0, max(0, totalLines-viewportHeight)).Draw(rt, "scrollOffset")

		cfg := ScrollbarConfig{
			TotalLines:     totalLines,
			ViewportHeight: viewportHeight,
			ScrollOffset:   scrollOffset,
		}

		_, height := calculateThumbBounds(cfg)

		// Height should always be at least 1 for valid configs
		require.GreaterOrEqual(t, height, 1, "thumb height should be >= 1 for valid config")
	})
}

func TestProperty_ThumbEndWithinViewport(t *testing.T) {
	// Property 3: start + height <= viewportHeight
	rapid.Check(t, func(rt *rapid.T) {
		totalLines := rapid.IntRange(1, 10000).Draw(rt, "totalLines")
		viewportHeight := rapid.IntRange(1, 100).Draw(rt, "viewportHeight")
		scrollOffset := rapid.IntRange(0, max(0, totalLines-viewportHeight)).Draw(rt, "scrollOffset")

		cfg := ScrollbarConfig{
			TotalLines:     totalLines,
			ViewportHeight: viewportHeight,
			ScrollOffset:   scrollOffset,
		}

		start, height := calculateThumbBounds(cfg)

		// Thumb end should not exceed viewport
		thumbEnd := start + height
		require.LessOrEqual(t, thumbEnd, viewportHeight, "thumb should not exceed viewport: start=%d, height=%d, viewport=%d", start, height, viewportHeight)
	})
}

func TestProperty_RenderScrollbarLineCount(t *testing.T) {
	// Property: RenderScrollbar should always return viewportHeight lines (or empty)
	rapid.Check(t, func(rt *rapid.T) {
		totalLines := rapid.IntRange(0, 1000).Draw(rt, "totalLines")
		viewportHeight := rapid.IntRange(0, 50).Draw(rt, "viewportHeight")
		scrollOffset := rapid.IntRange(0, max(0, totalLines-viewportHeight)).Draw(rt, "scrollOffset")

		cfg := ScrollbarConfig{
			TotalLines:     totalLines,
			ViewportHeight: viewportHeight,
			ScrollOffset:   scrollOffset,
		}

		result := RenderScrollbar(cfg)

		if totalLines <= 0 || viewportHeight <= 0 {
			require.Empty(t, result, "invalid config should return empty string")
			return
		}

		lines := strings.Split(result, "\n")
		require.Len(t, lines, viewportHeight, "should have exactly viewportHeight lines")
	})
}

// ===========================================================================
// Golden Tests for RenderScrollbar
// Run with -update flag to update golden files: go test -update ./internal/ui/shared/diffviewer/...
// ===========================================================================

func TestRenderScrollbar_Golden_ShortDiff(t *testing.T) {
	// Short diff (10 lines) - content smaller than viewport, no scrollbar needed
	cfg := ScrollbarConfig{
		TotalLines:     10,
		ViewportHeight: 30,
		ScrollOffset:   0,
		TrackChar:      "░",
		ThumbChar:      "█",
	}
	output := RenderScrollbar(cfg)
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestRenderScrollbar_Golden_MediumDiff(t *testing.T) {
	// Medium diff (100 lines) - scrollbar with thumb visible
	cfg := ScrollbarConfig{
		TotalLines:     100,
		ViewportHeight: 30,
		ScrollOffset:   35, // Scrolled to roughly middle
		TrackChar:      "░",
		ThumbChar:      "█",
	}
	output := RenderScrollbar(cfg)
	teatest.RequireEqualOutput(t, []byte(output))
}

func TestRenderScrollbar_Golden_LargeDiff(t *testing.T) {
	// Large diff (1000 lines) - simple scrollbar with small thumb
	cfg := ScrollbarConfig{
		TotalLines:     1000,
		ViewportHeight: 30,
		ScrollOffset:   485, // Near middle of file
		TrackChar:      "░",
		ThumbChar:      "█",
	}
	output := RenderScrollbar(cfg)
	teatest.RequireEqualOutput(t, []byte(output))
}
