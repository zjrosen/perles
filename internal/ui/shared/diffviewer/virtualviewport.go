package diffviewer

import (
	"strings"
)

// VirtualViewport renders only visible lines without padding overhead.
// This component bypasses the bubbles/viewport SetContent() path which creates
// O(n) strings for n total lines. Instead, it renders O(visible) lines directly.
//
// This eliminates the primary performance bottleneck identified in profiling:
// - Previously: 22K-element slice + 22K-line string per frame (1.6MB)
// - Now: ~50 visible lines per frame (~50KB)
type VirtualViewport struct {
	// vc holds the virtual content with all lines and rendering logic
	vc *VirtualContent

	// scrollOffset is the current scroll position (first visible line index)
	scrollOffset int

	// height is the number of visible lines in the viewport
	height int

	// width is the viewport width for line rendering
	width int

	// bufferLines is the number of lines to pre-render above/below visible area
	// for smoother scrolling and cache warming
	bufferLines int
}

// NewVirtualViewport creates a new VirtualViewport from a VirtualContent.
// The VirtualContent holds all lines and handles individual line rendering.
func NewVirtualViewport(vc *VirtualContent) *VirtualViewport {
	return &VirtualViewport{
		vc:           vc,
		scrollOffset: 0,
		height:       0,
		width:        0,
		bufferLines:  vc.BufferLines,
	}
}

// NewVirtualViewportFromFiles creates a VirtualViewport directly from diff files.
// This is a convenience constructor that creates the VirtualContent internally.
func NewVirtualViewportFromFiles(files []DiffFile, config VirtualContentConfig) *VirtualViewport {
	vc := NewVirtualContent(files, config)
	return NewVirtualViewport(vc)
}

// SetSize updates the viewport dimensions.
// Width changes invalidate the render cache since line rendering depends on width.
func (vv *VirtualViewport) SetSize(width, height int) {
	vv.width = width
	vv.height = height
	vv.vc.SetWidth(width)

	// Ensure scroll position is still valid after resize
	vv.clampScrollOffset()
}

// Height returns the current viewport height.
func (vv *VirtualViewport) Height() int {
	return vv.height
}

// Width returns the current viewport width.
func (vv *VirtualViewport) Width() int {
	return vv.width
}

// TotalLines returns the total number of lines in the content.
func (vv *VirtualViewport) TotalLines() int {
	return vv.vc.TotalLines
}

// YOffset returns the current scroll position (first visible line index).
func (vv *VirtualViewport) YOffset() int {
	return vv.scrollOffset
}

// SetYOffset sets the scroll position to the given offset.
// The offset is clamped to valid range [0, totalLines - height].
func (vv *VirtualViewport) SetYOffset(offset int) {
	vv.scrollOffset = offset
	vv.clampScrollOffset()
}

// ScrollUp scrolls up by n lines.
func (vv *VirtualViewport) ScrollUp(n int) {
	vv.scrollOffset -= n
	vv.clampScrollOffset()
}

// ScrollDown scrolls down by n lines.
func (vv *VirtualViewport) ScrollDown(n int) {
	vv.scrollOffset += n
	vv.clampScrollOffset()
}

// GotoTop scrolls to the top of the content.
func (vv *VirtualViewport) GotoTop() {
	vv.scrollOffset = 0
}

// GotoBottom scrolls to the bottom of the content.
func (vv *VirtualViewport) GotoBottom() {
	vv.scrollOffset = vv.maxScrollOffset()
}

// HalfPageUp scrolls up by half a page.
func (vv *VirtualViewport) HalfPageUp() {
	vv.ScrollUp(vv.height / 2)
}

// HalfPageDown scrolls down by half a page.
func (vv *VirtualViewport) HalfPageDown() {
	vv.ScrollDown(vv.height / 2)
}

// PageUp scrolls up by one page.
func (vv *VirtualViewport) PageUp() {
	vv.ScrollUp(vv.height)
}

// PageDown scrolls down by one page.
func (vv *VirtualViewport) PageDown() {
	vv.ScrollDown(vv.height)
}

// AtTop returns true if scrolled to the top.
func (vv *VirtualViewport) AtTop() bool {
	return vv.scrollOffset == 0
}

// AtBottom returns true if scrolled to the bottom.
func (vv *VirtualViewport) AtBottom() bool {
	return vv.scrollOffset >= vv.maxScrollOffset()
}

// ScrollPercent returns the scroll position as a percentage (0.0 to 1.0).
// Returns 0.0 if content fits within viewport.
func (vv *VirtualViewport) ScrollPercent() float64 {
	maxOffset := vv.maxScrollOffset()
	if maxOffset <= 0 {
		return 0.0
	}
	return float64(vv.scrollOffset) / float64(maxOffset)
}

// VisibleLineCount returns the number of lines currently visible.
// This may be less than height if near the end of content.
func (vv *VirtualViewport) VisibleLineCount() int {
	if vv.vc.TotalLines == 0 {
		return 0
	}
	endIdx := min(vv.scrollOffset+vv.height, vv.vc.TotalLines)
	return endIdx - vv.scrollOffset
}

// PastEnd returns true if the content is shorter than the viewport.
// In this case, there's no scrolling needed.
func (vv *VirtualViewport) PastEnd() bool {
	return vv.vc.TotalLines <= vv.height
}

// Render returns ONLY the visible lines as a single string.
// This is the key optimization: no padding strings, just visible content.
//
// The method also pre-warms the cache for lines in the buffer zone
// above and below the visible area for smoother scrolling.
func (vv *VirtualViewport) Render() string {
	if vv.vc.TotalLines == 0 || vv.height <= 0 || vv.width <= 0 {
		return ""
	}

	// Calculate visible range
	startIdx := vv.scrollOffset
	endIdx := min(startIdx+vv.height, vv.vc.TotalLines)

	// Pre-warm cache for buffer zone (above and below visible area)
	// This is done before rendering visible lines for smoother scrolling
	vv.prewarmCache(startIdx, endIdx)

	// Build the output string with only visible lines
	var sb strings.Builder
	// Pre-allocate: estimate ~100 chars per line average (including ANSI codes)
	sb.Grow(vv.height * 100)

	for i := startIdx; i < endIdx; i++ {
		if i > startIdx {
			sb.WriteByte('\n')
		}
		rendered := vv.vc.renderLine(i)
		sb.WriteString(rendered)
	}

	return sb.String()
}

// prewarmCache renders lines in the buffer zone to warm the cache.
// This happens asynchronously conceptually but inline here since
// the cache makes subsequent renders fast.
func (vv *VirtualViewport) prewarmCache(visibleStart, visibleEnd int) {
	// Pre-warm lines above visible area
	bufferStart := max(0, visibleStart-vv.bufferLines)
	for i := bufferStart; i < visibleStart; i++ {
		// Just call renderLine - it will cache the result
		_ = vv.vc.renderLine(i)
	}

	// Pre-warm lines below visible area
	bufferEnd := min(vv.vc.TotalLines, visibleEnd+vv.bufferLines)
	for i := visibleEnd; i < bufferEnd; i++ {
		_ = vv.vc.renderLine(i)
	}
}

// InvalidateCache clears the render cache, forcing re-render of all lines.
// Call this when theme or other visual properties change.
func (vv *VirtualViewport) InvalidateCache() {
	vv.vc.InvalidateCache()
}

// SetTheme updates the theme and invalidates cache if changed.
// Returns true if cache was cleared.
func (vv *VirtualViewport) SetTheme(theme string) bool {
	return vv.vc.SetTheme(theme)
}

// SetViewMode updates the view mode and invalidates cache if changed.
// Returns true if cache was cleared.
func (vv *VirtualViewport) SetViewMode(mode ViewMode) bool {
	return vv.vc.SetViewMode(mode)
}

// GetCacheMetrics returns cache performance metrics.
func (vv *VirtualViewport) GetCacheMetrics() CacheMetrics {
	return vv.vc.GetCacheMetrics()
}

// GetCacheHitRate returns the cache hit rate as a percentage (0-100).
func (vv *VirtualViewport) GetCacheHitRate() float64 {
	return vv.vc.GetCacheHitRate()
}

// VirtualContent returns the underlying VirtualContent for direct access.
// Use sparingly - prefer the VirtualViewport methods.
func (vv *VirtualViewport) VirtualContent() *VirtualContent {
	return vv.vc
}

// clampScrollOffset ensures scrollOffset is within valid bounds.
func (vv *VirtualViewport) clampScrollOffset() {
	maxOffset := vv.maxScrollOffset()
	if vv.scrollOffset < 0 {
		vv.scrollOffset = 0
	} else if vv.scrollOffset > maxOffset {
		vv.scrollOffset = maxOffset
	}
}

// maxScrollOffset returns the maximum valid scroll offset.
// This is totalLines - height, but never negative.
func (vv *VirtualViewport) maxScrollOffset() int {
	if vv.vc.TotalLines <= vv.height {
		return 0
	}
	return vv.vc.TotalLines - vv.height
}

// LineRange returns the start and end indices of currently visible lines.
// End is exclusive (following Go slice convention).
func (vv *VirtualViewport) LineRange() (start, end int) {
	start = vv.scrollOffset
	end = min(start+vv.height, vv.vc.TotalLines)
	return start, end
}

// EnsureVisible scrolls the viewport to make the given line index visible.
// If the line is already visible, no scrolling occurs.
// Returns true if scrolling occurred.
func (vv *VirtualViewport) EnsureVisible(lineIndex int) bool {
	if lineIndex < 0 || lineIndex >= vv.vc.TotalLines {
		return false
	}

	oldOffset := vv.scrollOffset

	// If line is above visible area, scroll up to show it
	if lineIndex < vv.scrollOffset {
		vv.scrollOffset = lineIndex
	}

	// If line is below visible area, scroll down to show it
	if lineIndex >= vv.scrollOffset+vv.height {
		vv.scrollOffset = lineIndex - vv.height + 1
	}

	vv.clampScrollOffset()
	return vv.scrollOffset != oldOffset
}

// ScrollToLine scrolls to put the given line at the top of the viewport.
func (vv *VirtualViewport) ScrollToLine(lineIndex int) {
	vv.scrollOffset = lineIndex
	vv.clampScrollOffset()
}

// ScrollToPercent scrolls to a position given as a percentage (0.0 to 1.0).
func (vv *VirtualViewport) ScrollToPercent(percent float64) {
	if percent < 0.0 {
		percent = 0.0
	} else if percent > 1.0 {
		percent = 1.0
	}
	maxOffset := vv.maxScrollOffset()
	vv.scrollOffset = int(percent * float64(maxOffset))
}
