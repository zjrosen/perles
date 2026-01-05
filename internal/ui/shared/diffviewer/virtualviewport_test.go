package diffviewer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// createTestVirtualViewport creates a VirtualViewport with n test lines.
func createTestVirtualViewport(t *testing.T, numLines int) *VirtualViewport {
	t.Helper()

	// Create a single file with numLines of context lines
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lines[i] = DiffLine{
			Type:       LineContext,
			OldLineNum: i + 1,
			NewLineNum: i + 1,
			Content:    strings.Repeat("x", 20), // 20 chars of content per line
		}
	}

	file := DiffFile{
		NewPath: "test.go",
		OldPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,10 +1,10 @@",
				Lines:  lines,
			},
		},
	}

	config := VirtualContentConfig{
		CacheCapacity: 100,
		CacheMaxBytes: 1024 * 1024, // 1MB
		BufferLines:   10,
	}

	return NewVirtualViewportFromFiles([]DiffFile{file}, config)
}

func TestVirtualViewport_NewVirtualViewport(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)

	require.NotNil(t, vv)
	require.Equal(t, 100, vv.TotalLines())
	require.Equal(t, 0, vv.YOffset())
	require.Equal(t, 0, vv.Height())
	require.Equal(t, 0, vv.Width())
}

func TestVirtualViewport_SetSize(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)

	vv.SetSize(80, 25)

	require.Equal(t, 80, vv.Width())
	require.Equal(t, 25, vv.Height())
}

func TestVirtualViewport_ScrollDown(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Scroll down 10 lines
	vv.ScrollDown(10)
	require.Equal(t, 10, vv.YOffset())

	// Scroll down more
	vv.ScrollDown(20)
	require.Equal(t, 30, vv.YOffset())
}

func TestVirtualViewport_ScrollUp(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Start at position 50
	vv.SetYOffset(50)
	require.Equal(t, 50, vv.YOffset())

	// Scroll up 10 lines
	vv.ScrollUp(10)
	require.Equal(t, 40, vv.YOffset())

	// Scroll up more
	vv.ScrollUp(20)
	require.Equal(t, 20, vv.YOffset())
}

func TestVirtualViewport_ScrollClampingAtTop(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Start at position 5
	vv.SetYOffset(5)

	// Scroll up 10 - should clamp to 0
	vv.ScrollUp(10)
	require.Equal(t, 0, vv.YOffset())
	require.True(t, vv.AtTop())
}

func TestVirtualViewport_ScrollClampingAtBottom(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Max offset for 100 lines with 25 height = 75
	maxOffset := 100 - 25

	// Scroll down past end
	vv.ScrollDown(200)
	require.Equal(t, maxOffset, vv.YOffset())
	require.True(t, vv.AtBottom())
}

func TestVirtualViewport_SetYOffset(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	vv.SetYOffset(50)
	require.Equal(t, 50, vv.YOffset())

	// Negative should clamp to 0
	vv.SetYOffset(-10)
	require.Equal(t, 0, vv.YOffset())

	// Past end should clamp
	vv.SetYOffset(1000)
	require.Equal(t, 75, vv.YOffset()) // 100 - 25 = 75
}

func TestVirtualViewport_GotoTopAndBottom(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Start in middle
	vv.SetYOffset(50)

	// Go to top
	vv.GotoTop()
	require.Equal(t, 0, vv.YOffset())
	require.True(t, vv.AtTop())

	// Go to bottom
	vv.GotoBottom()
	require.Equal(t, 75, vv.YOffset())
	require.True(t, vv.AtBottom())
}

func TestVirtualViewport_HalfPageScroll(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 20)

	// Half page down should move 10 lines
	vv.HalfPageDown()
	require.Equal(t, 10, vv.YOffset())

	// Half page up should move back 10 lines
	vv.HalfPageUp()
	require.Equal(t, 0, vv.YOffset())
}

func TestVirtualViewport_PageScroll(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 20)

	// Page down should move 20 lines
	vv.PageDown()
	require.Equal(t, 20, vv.YOffset())

	// Page up should move back 20 lines
	vv.PageUp()
	require.Equal(t, 0, vv.YOffset())
}

func TestVirtualViewport_ScrollPercent(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// At top, percent should be 0
	require.Equal(t, 0.0, vv.ScrollPercent())

	// At bottom, percent should be 1
	vv.GotoBottom()
	require.Equal(t, 1.0, vv.ScrollPercent())

	// At middle
	vv.SetYOffset(37) // Roughly half of 75 (max offset)
	percent := vv.ScrollPercent()
	require.InDelta(t, 0.493, percent, 0.01)
}

func TestVirtualViewport_ScrollPercent_ContentFitsInViewport(t *testing.T) {
	vv := createTestVirtualViewport(t, 10)
	vv.SetSize(80, 25) // Viewport bigger than content

	// Should return 0 when no scrolling is possible
	require.Equal(t, 0.0, vv.ScrollPercent())
	require.True(t, vv.PastEnd())
}

func TestVirtualViewport_VisibleLineCount(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// At top, should see 25 lines
	require.Equal(t, 25, vv.VisibleLineCount())

	// At bottom, should still see 25 lines
	vv.GotoBottom()
	require.Equal(t, 25, vv.VisibleLineCount())
}

func TestVirtualViewport_VisibleLineCount_SmallContent(t *testing.T) {
	vv := createTestVirtualViewport(t, 10)
	vv.SetSize(80, 25)

	// Should see all 10 lines
	require.Equal(t, 10, vv.VisibleLineCount())
}

func TestVirtualViewport_LineRange(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	start, end := vv.LineRange()
	require.Equal(t, 0, start)
	require.Equal(t, 25, end)

	vv.SetYOffset(50)
	start, end = vv.LineRange()
	require.Equal(t, 50, start)
	require.Equal(t, 75, end)

	vv.GotoBottom()
	start, end = vv.LineRange()
	require.Equal(t, 75, start)
	require.Equal(t, 100, end)
}

func TestVirtualViewport_Render_EmptyContent(t *testing.T) {
	vv := createTestVirtualViewport(t, 0)
	vv.SetSize(80, 25)

	result := vv.Render()
	require.Equal(t, "", result)
}

func TestVirtualViewport_Render_ZeroSize(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)

	// Zero height
	vv.SetSize(80, 0)
	require.Equal(t, "", vv.Render())

	// Zero width
	vv.SetSize(0, 25)
	require.Equal(t, "", vv.Render())
}

func TestVirtualViewport_Render_ProducesCorrectLineCount(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	result := vv.Render()

	// Count lines in output
	lineCount := strings.Count(result, "\n") + 1
	require.Equal(t, 25, lineCount)
}

func TestVirtualViewport_Render_AtDifferentPositions(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 10)

	// Render at top
	result1 := vv.Render()
	require.NotEmpty(t, result1)

	// Render at middle
	vv.SetYOffset(50)
	result2 := vv.Render()
	require.NotEmpty(t, result2)

	// Render at bottom
	vv.GotoBottom()
	result3 := vv.Render()
	require.NotEmpty(t, result3)

	// All renders should have same line count
	require.Equal(t, 10, strings.Count(result1, "\n")+1)
	require.Equal(t, 10, strings.Count(result2, "\n")+1)
	require.Equal(t, 10, strings.Count(result3, "\n")+1)
}

func TestVirtualViewport_Render_SmallContent(t *testing.T) {
	vv := createTestVirtualViewport(t, 5)
	vv.SetSize(80, 25)

	result := vv.Render()

	// Should only render 5 lines
	lineCount := strings.Count(result, "\n") + 1
	require.Equal(t, 5, lineCount)
}

func TestVirtualViewport_Render_NoExtraPadding(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 10)

	// Scroll to middle
	vv.SetYOffset(50)

	result := vv.Render()

	// The result should NOT contain empty lines at start or end (no padding)
	lines := strings.Split(result, "\n")
	require.Len(t, lines, 10)

	// First and last lines should not be empty
	require.NotEmpty(t, lines[0], "First line should not be empty padding")
	require.NotEmpty(t, lines[len(lines)-1], "Last line should not be empty padding")
}

func TestVirtualViewport_EnsureVisible(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Line 10 is already visible, no scroll needed
	scrolled := vv.EnsureVisible(10)
	require.False(t, scrolled)
	require.Equal(t, 0, vv.YOffset())

	// Line 50 is not visible, should scroll
	scrolled = vv.EnsureVisible(50)
	require.True(t, scrolled)
	// Line 50 should now be visible
	start, end := vv.LineRange()
	require.LessOrEqual(t, start, 50)
	require.Greater(t, end, 50)

	// Line 0 is not visible now, should scroll up
	scrolled = vv.EnsureVisible(0)
	require.True(t, scrolled)
	require.Equal(t, 0, vv.YOffset())
}

func TestVirtualViewport_EnsureVisible_InvalidIndex(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Negative index
	scrolled := vv.EnsureVisible(-1)
	require.False(t, scrolled)

	// Past end
	scrolled = vv.EnsureVisible(1000)
	require.False(t, scrolled)
}

func TestVirtualViewport_ScrollToLine(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	vv.ScrollToLine(50)
	require.Equal(t, 50, vv.YOffset())

	// Should clamp
	vv.ScrollToLine(90)
	require.Equal(t, 75, vv.YOffset()) // Clamped to max offset
}

func TestVirtualViewport_ScrollToPercent(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// 0% = top
	vv.ScrollToPercent(0.0)
	require.Equal(t, 0, vv.YOffset())

	// 100% = bottom
	vv.ScrollToPercent(1.0)
	require.Equal(t, 75, vv.YOffset())

	// 50% = middle
	vv.ScrollToPercent(0.5)
	require.Equal(t, 37, vv.YOffset()) // 75 * 0.5 = 37.5 -> 37

	// Should clamp invalid values
	vv.ScrollToPercent(-0.5)
	require.Equal(t, 0, vv.YOffset())

	vv.ScrollToPercent(1.5)
	require.Equal(t, 75, vv.YOffset())
}

func TestVirtualViewport_Cache(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// First render
	_ = vv.Render()
	metrics1 := vv.GetCacheMetrics()
	misses1 := metrics1.Misses

	// Second render at same position - should use cache
	_ = vv.Render()
	metrics2 := vv.GetCacheMetrics()

	// Hits should increase, misses should stay same
	require.Greater(t, metrics2.Hits, metrics1.Hits)
	require.Equal(t, misses1, metrics2.Misses)

	// Hit rate should improve
	hitRate := vv.GetCacheHitRate()
	require.Greater(t, hitRate, 0.0)
}

func TestVirtualViewport_InvalidateCache(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Render to populate cache
	_ = vv.Render()
	metrics1 := vv.GetCacheMetrics()
	require.Greater(t, metrics1.Misses, uint64(0))

	// Invalidate
	vv.InvalidateCache()

	// Render again - should have new misses
	_ = vv.Render()
	metrics2 := vv.GetCacheMetrics()

	// Misses should have increased (cache was cleared)
	require.Greater(t, metrics2.Misses, metrics1.Misses)
}

func TestVirtualViewport_SetTheme(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Render to populate cache
	_ = vv.Render()

	// Change theme - should clear cache
	changed := vv.SetTheme("dark")
	require.True(t, changed)

	// Setting same theme again should not clear cache
	changed = vv.SetTheme("dark")
	require.False(t, changed)
}

func TestVirtualViewport_SetViewMode(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Change view mode
	changed := vv.SetViewMode(ViewModeSideBySide)
	require.True(t, changed)

	// Setting same mode again should not clear cache
	changed = vv.SetViewMode(ViewModeSideBySide)
	require.False(t, changed)
}

func TestVirtualViewport_ResizeClampScrollPosition(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)
	vv.SetSize(80, 25)

	// Scroll to bottom
	vv.GotoBottom()
	require.Equal(t, 75, vv.YOffset())

	// Make viewport taller - should clamp scroll offset
	vv.SetSize(80, 50)

	// Max offset is now 100 - 50 = 50
	require.Equal(t, 50, vv.YOffset())
}

func TestVirtualViewport_VirtualContent(t *testing.T) {
	vv := createTestVirtualViewport(t, 100)

	vc := vv.VirtualContent()
	require.NotNil(t, vc)
	require.Equal(t, 100, vc.TotalLines)
}

// Benchmark to verify performance improvement
func BenchmarkVirtualViewport_Render_100Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(100)
	vv.SetSize(80, 25)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vv.Render()
	}
}

func BenchmarkVirtualViewport_Render_1000Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(1000)
	vv.SetSize(80, 25)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vv.Render()
	}
}

func BenchmarkVirtualViewport_Render_10000Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(10000)
	vv.SetSize(80, 25)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vv.Render()
	}
}

func BenchmarkVirtualViewport_Render_22000Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(22000)
	vv.SetSize(80, 25)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = vv.Render()
	}
}

func BenchmarkVirtualViewport_Render_ScrollMiddle_22000Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(22000)
	vv.SetSize(80, 25)
	vv.SetYOffset(11000) // Scroll to middle

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = vv.Render()
	}
}

func BenchmarkVirtualViewport_ScrollAndRender_22000Lines(b *testing.B) {
	vv := createTestVirtualViewportBench(22000)
	vv.SetSize(80, 25)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Simulate scrolling pattern
		vv.ScrollDown(5)
		if vv.AtBottom() {
			vv.GotoTop()
		}
		_ = vv.Render()
	}
}

// Helper for benchmarks
func createTestVirtualViewportBench(numLines int) *VirtualViewport {
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lines[i] = DiffLine{
			Type:       LineContext,
			OldLineNum: i + 1,
			NewLineNum: i + 1,
			Content:    strings.Repeat("x", 50),
		}
	}

	file := DiffFile{
		NewPath: "test.go",
		OldPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,10 +1,10 @@",
				Lines:  lines,
			},
		},
	}

	config := VirtualContentConfig{
		CacheCapacity: 1000,
		CacheMaxBytes: 10 * 1024 * 1024,
		BufferLines:   50,
	}

	return NewVirtualViewportFromFiles([]DiffFile{file}, config)
}
