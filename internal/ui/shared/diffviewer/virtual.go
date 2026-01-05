package diffviewer

import (
	"container/list"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// Package-level styles for hot path rendering to avoid allocations per frame.
// These are used in doRenderLine() and renderFileHeaderLine().
var (
	// Line type styles for diff rendering
	lineAddStyle     = lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
	lineDelStyle     = lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)
	lineContextStyle = lipgloss.NewStyle().Foreground(styles.DiffContextColor)
	lineHunkStyle    = lipgloss.NewStyle().Foreground(styles.DiffHunkColor)
	lineGutterStyle  = lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// File header styles
	fileHeaderNameStyle = lipgloss.NewStyle().
				Foreground(styles.TextPrimaryColor).
				Bold(true).
				Background(styles.SelectionBackgroundColor)
	fileHeaderBinaryStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Side-by-side specific styles
	sbsSeparatorStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	sbsEmptyStyle     = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
)

// Side-by-side rendering constants (matching renderer.go)
const (
	sbsSeparator   = "│"
	sbsGutterWidth = 5  // "NNNN " for line numbers
	sbsMinColWidth = 40 // Minimum width for each column
)

// VirtualContent manages virtual scrolling for large diffs.
// It only renders visible lines plus a buffer, using an LRU cache for efficiency.
type VirtualContent struct {
	// Lines is the source data - all diff lines across all hunks (unified view)
	Lines []VirtualLine

	// SideBySideLines is the pre-computed aligned pairs for side-by-side view
	// This is computed once from Lines when the content is created
	SideBySideLines []SideBySideLine

	// TotalLines is the total number of lines for the current view mode
	TotalLines int

	// UnifiedTotalLines is the count for unified view (always stored)
	UnifiedTotalLines int

	// SideBySideTotalLines is the count for side-by-side view (always stored)
	SideBySideTotalLines int

	// VisibleStart is the first visible line index
	VisibleStart int

	// VisibleEnd is the last visible line index (exclusive)
	VisibleEnd int

	// RenderCache caches rendered line strings
	RenderCache *RenderCache

	// Width is the current render width
	Width int

	// BufferLines is the number of lines to pre-render above/below visible area
	BufferLines int

	// ViewMode is the current display mode (unified/side-by-side)
	ViewMode ViewMode

	// Theme is the current theme identifier
	Theme string
}

// VirtualLine represents a single renderable line in the virtual content.
// It stores the raw data needed to render the line on demand.
type VirtualLine struct {
	Type         LineType
	OldLineNum   int
	NewLineNum   int
	Content      string
	HunkHeader   string // Only set for LineHunkHeader type
	FileHeader   string // Only set for file header lines (internal use)
	IsFileHeader bool   // True if this is a file header separator
	FileHash     string // Hash/ID for the source file (for cache key)
	HunkIndex    int    // Index of the hunk within the file (for cache key)
}

// SideBySideLine represents a single row in side-by-side diff view.
// Each row contains aligned left (old) and right (new) content.
type SideBySideLine struct {
	Left         *DiffLine // Line from old file (nil for insertion-only row)
	Right        *DiffLine // Line from new file (nil for deletion-only row)
	IsHunkHeader bool      // True if this is a hunk header row
	HunkHeader   string    // Hunk header text (only set if IsHunkHeader)
	IsFileHeader bool      // True if this is a file header separator
	FileHeader   string    // File header text (only set if IsFileHeader)
	FileHash     string    // Hash/ID for the source file (for cache key)
	HunkIndex    int       // Index of the hunk within the file (for cache key)
}

// PairType returns the type of this side-by-side row for styling.
func (sbl *SideBySideLine) PairType() string {
	if sbl.IsHunkHeader {
		return "hunk_header"
	}
	if sbl.IsFileHeader {
		return "file_header"
	}
	if sbl.Left == nil && sbl.Right == nil {
		return "empty"
	}
	if sbl.Left != nil && sbl.Right != nil {
		if sbl.Left.Type == LineContext && sbl.Right.Type == LineContext {
			return "context"
		}
		return "modification"
	}
	if sbl.Left != nil {
		return "deletion"
	}
	return "addition"
}

// RenderCacheKey uniquely identifies a rendered content block.
// Using FileHash+HunkIndex+Width+ViewMode+Theme allows for:
// - Proper invalidation when file content changes (FileHash)
// - Per-hunk caching for virtual scrolling (HunkIndex)
// - Re-render on width changes (Width)
// - Different renders for unified vs side-by-side (ViewMode)
// - Theme-aware caching (Theme)
type RenderCacheKey struct {
	FileHash  string   // SHA of file content or path for identity
	HunkIndex int      // Index of hunk within the file (-1 for file headers)
	LineIndex int      // Index of line within visible content
	Width     int      // Current render width
	ViewMode  ViewMode // Unified or side-by-side
	Theme     string   // Current theme identifier
}

// CacheMetrics tracks cache performance statistics.
type CacheMetrics struct {
	Hits       uint64 // Number of cache hits
	Misses     uint64 // Number of cache misses
	Evictions  uint64 // Number of entries evicted
	SizeEvicts uint64 // Number of evictions due to size limit
}

// HitRate returns the cache hit rate as a percentage (0-100).
// Returns 0 if no requests have been made.
func (m CacheMetrics) HitRate() float64 {
	total := m.Hits + m.Misses
	if total == 0 {
		return 0
	}
	return float64(m.Hits) / float64(total) * 100
}

// RenderCache is an LRU cache for rendered line strings.
// It has both item count and memory size limits for bounded resource usage.
// Eviction occurs when either limit is exceeded (whichever is hit first).
type RenderCache struct {
	capacity    int                              // Maximum number of entries
	maxBytes    int64                            // Maximum total bytes (~10MB default)
	currentSize int64                            // Current total size in bytes
	cache       map[RenderCacheKey]*list.Element // Key to LRU element mapping
	lru         *list.List                       // LRU list for eviction ordering
	mu          sync.Mutex                       // Thread safety

	// Render context (for invalidation)
	viewMode ViewMode // Current view mode
	theme    string   // Current theme identifier

	// Metrics
	Metrics CacheMetrics
}

// cacheEntry holds a cached rendered line with size tracking.
type cacheEntry struct {
	key   RenderCacheKey
	value string
	size  int64 // Approximate memory size of this entry
}

// entrySize estimates the memory usage of a cache entry.
// Includes the value string and a rough estimate for key fields.
func entrySize(key RenderCacheKey, value string) int64 {
	// String content
	size := int64(len(value))
	// Key fields: FileHash string + Theme string + fixed fields (~50 bytes)
	size += int64(len(key.FileHash))
	size += int64(len(key.Theme))
	size += 50 // Rough estimate for int fields, pointers, etc.
	return size
}

// DefaultMaxCacheBytes is the default memory limit for the cache (~10MB).
const DefaultMaxCacheBytes = 10 * 1024 * 1024

// NewRenderCache creates a new LRU cache with the given item capacity.
// Uses DefaultMaxCacheBytes for memory limit.
func NewRenderCache(capacity int) *RenderCache {
	return NewRenderCacheWithLimits(capacity, DefaultMaxCacheBytes)
}

// NewRenderCacheWithLimits creates a new LRU cache with both item and memory limits.
// Eviction occurs when either limit is exceeded.
func NewRenderCacheWithLimits(capacity int, maxBytes int64) *RenderCache {
	return &RenderCache{
		capacity:    capacity,
		maxBytes:    maxBytes,
		currentSize: 0,
		cache:       make(map[RenderCacheKey]*list.Element),
		lru:         list.New(),
	}
}

// Get retrieves a cached rendered line, returning ("", false) if not found.
// Updates metrics on each call.
func (c *RenderCache) Get(key RenderCacheKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		c.Metrics.Hits++
		return elem.Value.(*cacheEntry).value, true
	}
	c.Metrics.Misses++
	return "", false
}

// Put stores a rendered line in the cache.
// Evicts entries when either item count or memory limit is exceeded.
func (c *RenderCache) Put(key RenderCacheKey, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := entrySize(key, value)

	// Update existing entry
	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		oldEntry := elem.Value.(*cacheEntry)
		c.currentSize -= oldEntry.size
		oldEntry.value = value
		oldEntry.size = size
		c.currentSize += size
		return
	}

	// Evict until we have room for both count and size limits
	for c.lru.Len() >= c.capacity || (c.maxBytes > 0 && c.currentSize+size > c.maxBytes) {
		oldest := c.lru.Back()
		if oldest == nil {
			break
		}
		entry := oldest.Value.(*cacheEntry)
		delete(c.cache, entry.key)
		c.lru.Remove(oldest)
		c.currentSize -= entry.size
		c.Metrics.Evictions++
		if c.currentSize+size > c.maxBytes {
			c.Metrics.SizeEvicts++
		}
	}

	entry := &cacheEntry{key: key, value: value, size: size}
	elem := c.lru.PushFront(entry)
	c.cache[key] = elem
	c.currentSize += size
}

// Clear empties the cache but preserves metrics.
func (c *RenderCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[RenderCacheKey]*list.Element)
	c.lru.Init()
	c.currentSize = 0
}

// ResetMetrics clears the performance metrics.
func (c *RenderCache) ResetMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Metrics = CacheMetrics{}
}

// Size returns the current number of cached entries.
func (c *RenderCache) Size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// ByteSize returns the current estimated memory usage in bytes.
func (c *RenderCache) ByteSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentSize
}

// GetMetrics returns a copy of the current cache metrics.
func (c *RenderCache) GetMetrics() CacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Metrics
}

// SetViewMode updates the view mode and clears cache if it changed.
// Returns true if cache was cleared.
func (c *RenderCache) SetViewMode(mode ViewMode) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.viewMode != mode {
		c.viewMode = mode
		c.clearLocked()
		return true
	}
	return false
}

// SetTheme updates the theme and clears cache if it changed.
// Returns true if cache was cleared.
func (c *RenderCache) SetTheme(theme string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.theme != theme {
		c.theme = theme
		c.clearLocked()
		return true
	}
	return false
}

// ViewMode returns the current view mode.
func (c *RenderCache) ViewMode() ViewMode {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.viewMode
}

// Theme returns the current theme identifier.
func (c *RenderCache) Theme() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.theme
}

// clearLocked clears the cache (must be called with lock held).
func (c *RenderCache) clearLocked() {
	c.cache = make(map[RenderCacheKey]*list.Element)
	c.lru.Init()
	c.currentSize = 0
}

// VirtualContentConfig holds configuration for virtual content creation.
type VirtualContentConfig struct {
	// CacheCapacity is the max number of rendered lines to cache (default: 1000)
	CacheCapacity int
	// CacheMaxBytes is the max memory for cache in bytes (default: 10MB)
	CacheMaxBytes int64
	// BufferLines is the number of lines to pre-render above/below visible area (default: 50)
	BufferLines int
}

// DefaultVirtualContentConfig returns sensible defaults.
func DefaultVirtualContentConfig() VirtualContentConfig {
	return VirtualContentConfig{
		CacheCapacity: 1000,
		CacheMaxBytes: DefaultMaxCacheBytes,
		BufferLines:   50,
	}
}

// NewVirtualContent creates a new VirtualContent from diff files.
// It pre-computes both unified and side-by-side line arrays for fast view switching.
func NewVirtualContent(files []DiffFile, config VirtualContentConfig) *VirtualContent {
	if config.CacheCapacity <= 0 {
		config.CacheCapacity = 1000
	}
	if config.CacheMaxBytes <= 0 {
		config.CacheMaxBytes = DefaultMaxCacheBytes
	}
	if config.BufferLines <= 0 {
		config.BufferLines = 50
	}

	vc := &VirtualContent{
		Lines:           make([]VirtualLine, 0),
		SideBySideLines: make([]SideBySideLine, 0),
		RenderCache:     NewRenderCacheWithLimits(config.CacheCapacity, config.CacheMaxBytes),
		BufferLines:     config.BufferLines,
	}

	// Convert files to virtual lines (unified view)
	for i, file := range files {
		// Generate a file hash from the path (could use content hash for true cache invalidation)
		fileHash := fileHashFromPath(file.NewPath, file.OldPath)

		// Add file header for multi-file views
		if len(files) > 1 {
			filename := file.NewPath
			if file.IsDeleted {
				filename = file.OldPath
			}
			vc.Lines = append(vc.Lines, VirtualLine{
				IsFileHeader: true,
				FileHeader:   filename,
				Content:      formatFileHeaderLine(filename, file),
				FileHash:     fileHash,
				HunkIndex:    -1, // File headers have hunk index -1
			})
		}

		// Add all hunks
		for hunkIdx, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				vl := VirtualLine{
					Type:       line.Type,
					OldLineNum: line.OldLineNum,
					NewLineNum: line.NewLineNum,
					Content:    line.Content,
					FileHash:   fileHash,
					HunkIndex:  hunkIdx,
				}
				if line.Type == LineHunkHeader {
					vl.HunkHeader = hunk.Header
				}
				vc.Lines = append(vc.Lines, vl)
			}
		}

		// Add empty line between files for multi-file views
		if len(files) > 1 && i < len(files)-1 {
			vc.Lines = append(vc.Lines, VirtualLine{
				Type:      LineContext,
				Content:   "",
				FileHash:  fileHash,
				HunkIndex: -2, // Separator lines have hunk index -2
			})
		}
	}

	// Pre-compute side-by-side aligned lines
	vc.SideBySideLines = buildSideBySideLines(files)

	// Store counts for both views
	vc.UnifiedTotalLines = len(vc.Lines)
	vc.SideBySideTotalLines = len(vc.SideBySideLines)

	// Default to unified view
	vc.TotalLines = vc.UnifiedTotalLines
	return vc
}

// buildSideBySideLines creates pre-aligned side-by-side lines from diff files.
// This converts the unified diff data into aligned pairs using the alignHunk algorithm.
func buildSideBySideLines(files []DiffFile) []SideBySideLine {
	var lines []SideBySideLine

	for i, file := range files {
		fileHash := fileHashFromPath(file.NewPath, file.OldPath)

		// Add file header for multi-file views
		if len(files) > 1 {
			filename := file.NewPath
			if file.IsDeleted {
				filename = file.OldPath
			}
			lines = append(lines, SideBySideLine{
				IsFileHeader: true,
				FileHeader:   formatFileHeaderLine(filename, file),
				FileHash:     fileHash,
				HunkIndex:    -1,
			})
		}

		// Convert each hunk to aligned pairs
		for hunkIdx, hunk := range file.Hunks {
			// Use the existing alignHunk function to pair lines
			pairs := alignHunk(hunk)

			for _, pair := range pairs {
				sbl := SideBySideLine{
					Left:      pair.Left,
					Right:     pair.Right,
					FileHash:  fileHash,
					HunkIndex: hunkIdx,
				}

				// Check for hunk header
				if pair.IsHunkHeader() {
					sbl.IsHunkHeader = true
					sbl.HunkHeader = hunk.Header
				}

				lines = append(lines, sbl)
			}
		}

		// Add empty line between files for multi-file views
		if len(files) > 1 && i < len(files)-1 {
			lines = append(lines, SideBySideLine{
				FileHash:  fileHash,
				HunkIndex: -2, // Separator
			})
		}
	}

	return lines
}

// NewVirtualContentFromFile creates virtual content from a single file.
func NewVirtualContentFromFile(file DiffFile, config VirtualContentConfig) *VirtualContent {
	return NewVirtualContent([]DiffFile{file}, config)
}

// SetVisibleRange updates the visible range based on viewport scroll position.
func (vc *VirtualContent) SetVisibleRange(scrollTop, viewportHeight int) {
	vc.VisibleStart = max(0, scrollTop-vc.BufferLines)
	vc.VisibleEnd = min(vc.TotalLines, scrollTop+viewportHeight+vc.BufferLines)
}

// SetWidth updates the render width and clears the cache if width changed.
func (vc *VirtualContent) SetWidth(width int) {
	if vc.Width != width {
		vc.Width = width
		vc.RenderCache.Clear()
	}
}

// RenderVisible generates the content string for the visible range.
// Returns a string suitable for setting as viewport content.
func (vc *VirtualContent) RenderVisible() string {
	if vc.TotalLines == 0 || vc.Width <= 0 {
		return ""
	}

	var lines []string

	// Add padding lines for content above visible range
	// This maintains correct scroll position in the viewport
	for i := 0; i < vc.VisibleStart; i++ {
		lines = append(lines, "")
	}

	// Render visible lines
	for i := vc.VisibleStart; i < vc.VisibleEnd && i < vc.TotalLines; i++ {
		rendered := vc.renderLine(i)
		lines = append(lines, rendered)
	}

	// Add padding lines for content below visible range
	for i := vc.VisibleEnd; i < vc.TotalLines; i++ {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderLine renders a single line, using cache when available.
// Dispatches to unified or side-by-side rendering based on view mode.
func (vc *VirtualContent) renderLine(index int) string {
	// Dispatch based on view mode
	if vc.ViewMode == ViewModeSideBySide {
		return vc.renderSideBySideLineAt(index)
	}
	return vc.renderUnifiedLineAt(index)
}

// renderUnifiedLineAt renders a unified view line at the given index.
func (vc *VirtualContent) renderUnifiedLineAt(index int) string {
	line := vc.Lines[index]

	// Build the enhanced cache key with all context
	key := RenderCacheKey{
		FileHash:  line.FileHash,
		HunkIndex: line.HunkIndex,
		LineIndex: index,
		Width:     vc.Width,
		ViewMode:  vc.ViewMode,
		Theme:     vc.Theme,
	}

	// Check cache
	if cached, ok := vc.RenderCache.Get(key); ok {
		return cached
	}

	// Render the line
	rendered := vc.doRenderLine(line)

	// Cache and return
	vc.RenderCache.Put(key, rendered)
	return rendered
}

// renderSideBySideLineAt renders a side-by-side view line at the given index.
func (vc *VirtualContent) renderSideBySideLineAt(index int) string {
	if index >= len(vc.SideBySideLines) {
		return ""
	}
	sbl := vc.SideBySideLines[index]

	// Build cache key
	key := RenderCacheKey{
		FileHash:  sbl.FileHash,
		HunkIndex: sbl.HunkIndex,
		LineIndex: index,
		Width:     vc.Width,
		ViewMode:  vc.ViewMode,
		Theme:     vc.Theme,
	}

	// Check cache
	if cached, ok := vc.RenderCache.Get(key); ok {
		return cached
	}

	// Render the side-by-side line
	rendered := vc.doRenderSideBySideLine(sbl)

	// Cache and return
	vc.RenderCache.Put(key, rendered)
	return rendered
}

// doRenderLine performs the actual rendering of a single line.
// Uses package-level styles to avoid allocations per frame.
func (vc *VirtualContent) doRenderLine(line VirtualLine) string {
	// Handle file headers
	if line.IsFileHeader {
		return vc.renderFileHeaderLine(line)
	}

	// Calculate content width (subtract gutter)
	gutterWidth := lineNumberWidth + 3 // "NNNN | "
	contentWidth := max(vc.Width-gutterWidth, 1)

	switch line.Type {
	case LineHunkHeader:
		headerText := line.HunkHeader
		if len(headerText) > vc.Width {
			headerText = ansi.Truncate(headerText, vc.Width, "...")
		}
		return lineHunkStyle.Render(headerText)

	case LineAddition:
		gutter := formatGutter(0, line.NewLineNum)
		prefix := "+"
		content := line.Content
		fullLine := prefix + content
		if len(fullLine) > contentWidth {
			fullLine = ansi.Truncate(fullLine, contentWidth, "")
		}
		return lineGutterStyle.Render(gutter) + lineAddStyle.Render(fullLine)

	case LineDeletion:
		gutter := formatGutter(line.OldLineNum, 0)
		prefix := "-"
		content := line.Content
		fullLine := prefix + content
		if len(fullLine) > contentWidth {
			fullLine = ansi.Truncate(fullLine, contentWidth, "")
		}
		return lineGutterStyle.Render(gutter) + lineDelStyle.Render(fullLine)

	case LineContext:
		gutter := formatGutter(line.OldLineNum, line.NewLineNum)
		prefix := " "
		content := line.Content
		fullLine := prefix + content
		if len(fullLine) > contentWidth {
			fullLine = ansi.Truncate(fullLine, contentWidth, "")
		}
		return lineGutterStyle.Render(gutter) + lineContextStyle.Render(fullLine)
	}

	return ""
}

// renderFileHeaderLine renders a file header separator line.
// The filename is highlighted with a background, but stats are rendered separately
// with color-coded additions/deletions.
// Uses package-level styles to avoid allocations per frame.
func (vc *VirtualContent) renderFileHeaderLine(line VirtualLine) string {
	// Parse filename and stats from content
	filename, adds, dels, isBinary := parseFileHeaderContent(line.Content)

	// Truncate filename if needed (leave room for stats)
	statsWidth := 0
	if isBinary {
		statsWidth = 8 // " binary"
	} else if adds > 0 || dels > 0 {
		statsWidth = 2 // space + approximate stats width
		if adds > 0 {
			statsWidth += len(itoa(adds)) + 2 // "+N "
		}
		if dels > 0 {
			statsWidth += len(itoa(dels)) + 2 // "-N"
		}
	}

	maxFilenameWidth := max(vc.Width-statsWidth, 10)
	if lipgloss.Width(filename) > maxFilenameWidth {
		filename = ansi.Truncate(filename, maxFilenameWidth, "…")
	}

	// Render filename with highlight using package-level style
	result := fileHeaderNameStyle.Render(filename)

	// Append stats with color-coded styling (no background)
	// Reuse line styles which have the same colors
	if isBinary {
		result += " " + fileHeaderBinaryStyle.Render("binary")
	} else {
		if adds > 0 {
			result += " " + lineAddStyle.Render("+"+itoa(adds))
		}
		if dels > 0 {
			result += " " + lineDelStyle.Render("-"+itoa(dels))
		}
	}

	return result
}

// doRenderSideBySideLine performs the actual rendering of a side-by-side row.
// Uses package-level styles to avoid allocations per frame.
func (vc *VirtualContent) doRenderSideBySideLine(sbl SideBySideLine) string {
	// Handle file headers
	if sbl.IsFileHeader {
		return vc.renderSideBySideFileHeader(sbl)
	}

	// Calculate column widths
	// Layout: [gutter][content] │ [gutter][content]
	// Each side gets half the width minus the separator
	sideWidth := (vc.Width - 1) / 2 // -1 for separator
	leftContentWidth := max(sideWidth-sbsGutterWidth, 1)
	rightContentWidth := max(sideWidth-sbsGutterWidth, 1)

	var leftSide, rightSide string

	switch sbl.PairType() {
	case "hunk_header":
		// Render hunk header spanning left side, empty on right
		headerText := sbl.HunkHeader
		if len(headerText) > sideWidth {
			headerText = ansi.Truncate(headerText, sideWidth, "...")
		}
		leftSide = lineHunkStyle.Render(padRightTo(headerText, sideWidth))
		rightSide = strings.Repeat(" ", sideWidth)

	case "context":
		// Both sides show the same content
		leftSide = vc.renderSideBySideColumn(sbl.Left, leftContentWidth, lineContextStyle)
		rightSide = vc.renderSideBySideColumn(sbl.Right, rightContentWidth, lineContextStyle)

	case "modification":
		// Left shows deletion, right shows addition
		leftSide = vc.renderSideBySideColumn(sbl.Left, leftContentWidth, lineDelStyle)
		rightSide = vc.renderSideBySideColumn(sbl.Right, rightContentWidth, lineAddStyle)

	case "deletion":
		// Left shows deletion, right is empty
		leftSide = vc.renderSideBySideColumn(sbl.Left, leftContentWidth, lineDelStyle)
		rightSide = vc.renderSideBySideEmptyColumn(sbsGutterWidth, rightContentWidth)

	case "addition":
		// Left is empty, right shows addition
		leftSide = vc.renderSideBySideEmptyColumn(sbsGutterWidth, leftContentWidth)
		rightSide = vc.renderSideBySideColumn(sbl.Right, rightContentWidth, lineAddStyle)

	case "empty":
		// Both sides empty (separator line)
		leftSide = strings.Repeat(" ", sideWidth)
		rightSide = strings.Repeat(" ", sideWidth)
	}

	return leftSide + sbsSeparatorStyle.Render(sbsSeparator) + rightSide
}

// renderSideBySideFileHeader renders a file header for side-by-side view.
func (vc *VirtualContent) renderSideBySideFileHeader(sbl SideBySideLine) string {
	// Parse filename and stats from content
	filename, adds, dels, isBinary := parseFileHeaderContent(sbl.FileHeader)

	// For side-by-side, render filename centered across full width
	result := fileHeaderNameStyle.Render(filename)

	// Append stats
	if isBinary {
		result += " " + fileHeaderBinaryStyle.Render("binary")
	} else {
		if adds > 0 {
			result += " " + lineAddStyle.Render("+"+itoa(adds))
		}
		if dels > 0 {
			result += " " + lineDelStyle.Render("-"+itoa(dels))
		}
	}

	// Pad to full width
	resultWidth := lipgloss.Width(result)
	if resultWidth < vc.Width {
		result += strings.Repeat(" ", vc.Width-resultWidth)
	}

	return result
}

// renderSideBySideColumn renders a single column (left or right) for side-by-side view.
func (vc *VirtualContent) renderSideBySideColumn(line *DiffLine, contentWidth int, lineStyle lipgloss.Style) string {
	if line == nil {
		return strings.Repeat(" ", sbsGutterWidth+contentWidth)
	}

	// Format gutter (line number)
	var lineNum int
	if line.Type == LineDeletion {
		lineNum = line.OldLineNum
	} else {
		lineNum = line.NewLineNum
	}

	gutter := formatSideBySideGutter(lineNum)

	// Render content
	content := line.Content
	contentStr := lineStyle.Render(content)

	// Truncate if needed (measure actual display width)
	displayWidth := lipgloss.Width(contentStr)
	if displayWidth > contentWidth {
		contentStr = ansi.Truncate(contentStr, contentWidth, "")
		displayWidth = lipgloss.Width(contentStr)
	}

	// Pad to exact width
	padding := contentWidth - displayWidth
	if padding > 0 {
		contentStr += strings.Repeat(" ", padding)
	}

	return lineGutterStyle.Render(gutter) + contentStr
}

// renderSideBySideEmptyColumn renders an empty column for side-by-side view.
func (vc *VirtualContent) renderSideBySideEmptyColumn(gutterWidth, contentWidth int) string {
	gutter := strings.Repeat(" ", gutterWidth)
	content := strings.Repeat(" ", contentWidth)
	return lineGutterStyle.Render(gutter) + sbsEmptyStyle.Render(content)
}

// formatSideBySideGutter formats a line number for side-by-side gutter (5 chars wide).
func formatSideBySideGutter(lineNum int) string {
	if lineNum == 0 {
		return "     "
	}
	s := itoa(lineNum)
	if len(s) < 4 {
		s = strings.Repeat(" ", 4-len(s)) + s
	}
	return s + " "
}

// padRightTo pads a string with spaces to reach the target width.
func padRightTo(s string, width int) string {
	currentWidth := lipgloss.Width(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}

// parseFileHeaderContent extracts filename and stats from the header content string.
func parseFileHeaderContent(content string) (filename string, adds, dels int, binary bool) {
	// Content format: "filename +N -M" or "filename binary" or just "filename"
	parts := strings.Split(content, " ")
	if len(parts) == 0 {
		return content, 0, 0, false
	}

	// Find where stats start (first part starting with + or - or "binary")
	statsStart := len(parts)
	for i, part := range parts {
		if part == "binary" || (len(part) > 0 && (part[0] == '+' || part[0] == '-')) {
			statsStart = i
			break
		}
	}

	filename = strings.Join(parts[:statsStart], " ")

	// Parse stats
	for i := statsStart; i < len(parts); i++ {
		part := parts[i]
		if part == "binary" {
			binary = true
		} else if len(part) > 1 && part[0] == '+' {
			adds = parseIntSimple(part[1:])
		} else if len(part) > 1 && part[0] == '-' {
			dels = parseIntSimple(part[1:])
		}
	}

	return filename, adds, dels, binary
}

// parseIntSimple parses an integer from a string without using strconv.
// Returns 0 if the string is empty or contains non-digit characters.
func parseIntSimple(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// formatFileHeaderLine creates the header text for a file.
func formatFileHeaderLine(filename string, file DiffFile) string {
	stats := formatStatsPlain(file.Additions, file.Deletions, file.IsBinary)
	if stats != "" {
		return filename + " " + stats
	}
	return filename
}

// formatStatsPlain formats stats without styling (for file headers in virtual content).
func formatStatsPlain(additions, deletions int, binary bool) string {
	if binary {
		return "binary"
	}

	var parts []string
	if additions > 0 {
		parts = append(parts, "+"+string(rune('0'+additions%10)))
		if additions >= 10 {
			parts[len(parts)-1] = "+" + itoa(additions)
		}
	}
	if deletions > 0 {
		del := "-" + itoa(deletions)
		parts = append(parts, del)
	}

	return strings.Join(parts, " ")
}

// itoa converts an int to string (simple helper to avoid fmt import in hot path).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// InvalidateCache clears the render cache, forcing re-render.
// Call this when theme or other visual properties change.
func (vc *VirtualContent) InvalidateCache() {
	vc.RenderCache.Clear()
}

// SetViewMode updates the view mode and clears cache if it changed.
// Also updates TotalLines to reflect the appropriate count for the view mode.
// Returns true if cache was cleared.
func (vc *VirtualContent) SetViewMode(mode ViewMode) bool {
	if vc.ViewMode != mode {
		vc.ViewMode = mode
		vc.RenderCache.Clear()

		// Update TotalLines based on view mode
		if mode == ViewModeSideBySide {
			vc.TotalLines = vc.SideBySideTotalLines
		} else {
			vc.TotalLines = vc.UnifiedTotalLines
		}
		return true
	}
	return false
}

// SetTheme updates the theme identifier and clears cache if it changed.
// Returns true if cache was cleared.
func (vc *VirtualContent) SetTheme(theme string) bool {
	if vc.Theme != theme {
		vc.Theme = theme
		vc.RenderCache.Clear()
		return true
	}
	return false
}

// GetCacheMetrics returns the current cache performance metrics.
func (vc *VirtualContent) GetCacheMetrics() CacheMetrics {
	return vc.RenderCache.GetMetrics()
}

// GetCacheHitRate returns the current cache hit rate as a percentage (0-100).
func (vc *VirtualContent) GetCacheHitRate() float64 {
	return vc.RenderCache.GetMetrics().HitRate()
}

// fileHashFromPath generates a simple hash string from file paths.
// Uses the path itself as the hash since content changes create new VirtualContent instances.
func fileHashFromPath(newPath, oldPath string) string {
	if newPath != "" && newPath != "/dev/null" {
		return newPath
	}
	if oldPath != "" && oldPath != "/dev/null" {
		return oldPath
	}
	return "unknown"
}
