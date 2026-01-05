package diffviewer

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderCache_Basic(t *testing.T) {
	cache := NewRenderCache(100)

	// Test Put and Get
	key := RenderCacheKey{LineIndex: 0, Width: 80}
	cache.Put(key, "line content")

	got, ok := cache.Get(key)
	require.True(t, ok)
	require.Equal(t, "line content", got)

	// Test miss
	missKey := RenderCacheKey{LineIndex: 1, Width: 80}
	_, ok = cache.Get(missKey)
	require.False(t, ok)
}

func TestRenderCache_LRUEviction(t *testing.T) {
	cache := NewRenderCache(3)

	// Fill cache to capacity
	cache.Put(RenderCacheKey{LineIndex: 0, Width: 80}, "line0")
	cache.Put(RenderCacheKey{LineIndex: 1, Width: 80}, "line1")
	cache.Put(RenderCacheKey{LineIndex: 2, Width: 80}, "line2")

	require.Equal(t, 3, cache.Size())

	// Access line0 to make it recently used
	cache.Get(RenderCacheKey{LineIndex: 0, Width: 80})

	// Add new entry - should evict line1 (least recently used)
	cache.Put(RenderCacheKey{LineIndex: 3, Width: 80}, "line3")

	require.Equal(t, 3, cache.Size())

	// line0 should still be there (was accessed)
	_, ok := cache.Get(RenderCacheKey{LineIndex: 0, Width: 80})
	require.True(t, ok)

	// line1 should be evicted
	_, ok = cache.Get(RenderCacheKey{LineIndex: 1, Width: 80})
	require.False(t, ok)

	// line2 and line3 should be there
	_, ok = cache.Get(RenderCacheKey{LineIndex: 2, Width: 80})
	require.True(t, ok)
	_, ok = cache.Get(RenderCacheKey{LineIndex: 3, Width: 80})
	require.True(t, ok)
}

func TestRenderCache_Clear(t *testing.T) {
	cache := NewRenderCache(10)

	cache.Put(RenderCacheKey{LineIndex: 0, Width: 80}, "line0")
	cache.Put(RenderCacheKey{LineIndex: 1, Width: 80}, "line1")
	require.Equal(t, 2, cache.Size())

	cache.Clear()
	require.Equal(t, 0, cache.Size())

	_, ok := cache.Get(RenderCacheKey{LineIndex: 0, Width: 80})
	require.False(t, ok)
}

func TestNewVirtualContent_SingleFile(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,4 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext, Content: "func foo() {", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "    newLine()", NewLineNum: 2},
					{Type: LineContext, Content: "}", OldLineNum: 2, NewLineNum: 3},
				},
			},
		},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())

	require.Equal(t, 4, vc.TotalLines)
	require.NotNil(t, vc.RenderCache)

	// Verify line types
	require.Equal(t, LineHunkHeader, vc.Lines[0].Type)
	require.Equal(t, LineContext, vc.Lines[1].Type)
	require.Equal(t, LineAddition, vc.Lines[2].Type)
	require.Equal(t, LineContext, vc.Lines[3].Type)
}

func TestNewVirtualContent_MultipleFiles(t *testing.T) {
	files := []DiffFile{
		{
			NewPath: "file1.go",
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,2 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader},
						{Type: LineAddition, Content: "new line", NewLineNum: 1},
					},
				},
			},
		},
		{
			NewPath: "file2.go",
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader},
						{Type: LineDeletion, Content: "old line", OldLineNum: 1},
					},
				},
			},
		},
	}

	vc := NewVirtualContent(files, DefaultVirtualContentConfig())

	// File header + 2 lines + empty separator + File header + 2 lines
	// file1.go (header) + hunk header + addition + empty line
	// file2.go (header) + hunk header + deletion
	// = 1 + 2 + 1 + 1 + 2 = 7
	require.Equal(t, 7, vc.TotalLines)

	// First should be file header
	require.True(t, vc.Lines[0].IsFileHeader)
	require.Equal(t, "file1.go", vc.Lines[0].FileHeader)
}

func TestVirtualContent_SetVisibleRange(t *testing.T) {
	// Create a virtual content with 100 lines
	lines := make([]DiffLine, 100)
	for i := 0; i < 100; i++ {
		lines[i] = DiffLine{Type: LineContext, Content: "line", OldLineNum: i + 1, NewLineNum: i + 1}
	}

	file := DiffFile{
		NewPath: "test.go",
		Hunks:   []DiffHunk{{Lines: lines}},
	}

	config := VirtualContentConfig{
		CacheCapacity: 100,
		BufferLines:   10,
	}
	vc := NewVirtualContent([]DiffFile{file}, config)

	// Test initial state
	vc.SetVisibleRange(0, 20)
	require.Equal(t, 0, vc.VisibleStart) // max(0, 0-10) = 0
	require.Equal(t, 30, vc.VisibleEnd)  // min(100, 0+20+10) = 30

	// Test scrolled position
	vc.SetVisibleRange(50, 20)
	require.Equal(t, 40, vc.VisibleStart) // max(0, 50-10) = 40
	require.Equal(t, 80, vc.VisibleEnd)   // min(100, 50+20+10) = 80

	// Test near end
	vc.SetVisibleRange(90, 20)
	require.Equal(t, 80, vc.VisibleStart) // max(0, 90-10) = 80
	require.Equal(t, 100, vc.VisibleEnd)  // min(100, 90+20+10) = 100
}

func TestVirtualContent_SetWidth(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext, Content: "line", OldLineNum: 1, NewLineNum: 1},
				},
			},
		},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(80)

	// Render a line to populate cache
	vc.SetVisibleRange(0, 10)
	_ = vc.RenderVisible()
	require.Greater(t, vc.RenderCache.Size(), 0)

	// Change width - should clear cache
	vc.SetWidth(100)
	require.Equal(t, 0, vc.RenderCache.Size())
}

func TestVirtualContent_RenderVisible(t *testing.T) {
	// Create a simple file with 5 lines
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,2 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext, Content: "context line", OldLineNum: 1, NewLineNum: 1},
					{Type: LineAddition, Content: "added line", NewLineNum: 2},
					{Type: LineDeletion, Content: "deleted line", OldLineNum: 2},
					{Type: LineContext, Content: "more context", OldLineNum: 3, NewLineNum: 3},
				},
			},
		},
	}

	config := VirtualContentConfig{
		CacheCapacity: 100,
		BufferLines:   1, // Small buffer for testing
	}
	vc := NewVirtualContent([]DiffFile{file}, config)
	vc.SetWidth(100)
	vc.SetVisibleRange(0, 5)

	content := vc.RenderVisible()
	lines := strings.Split(content, "\n")

	// Should have rendered 5 lines
	require.Len(t, lines, 5)

	// Check that hunk header is present
	require.Contains(t, lines[0], "@@")

	// Check that additions and deletions are present
	foundAddition := false
	foundDeletion := false
	for _, line := range lines {
		if strings.Contains(line, "+") && strings.Contains(line, "added") {
			foundAddition = true
		}
		if strings.Contains(line, "-") && strings.Contains(line, "deleted") {
			foundDeletion = true
		}
	}
	require.True(t, foundAddition, "Should contain addition line")
	require.True(t, foundDeletion, "Should contain deletion line")
}

func TestVirtualContent_LargeDiff(t *testing.T) {
	// Create a large diff (10K+ lines)
	numLines := 10000
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lineType := LineContext
		if i%10 == 0 {
			lineType = LineAddition
		} else if i%10 == 1 {
			lineType = LineDeletion
		}
		lines[i] = DiffLine{
			Type:       lineType,
			Content:    "line content here",
			OldLineNum: i + 1,
			NewLineNum: i + 1,
		}
	}

	file := DiffFile{
		NewPath: "large.go",
		Hunks:   []DiffHunk{{Header: "@@ -1,10000 +1,10000 @@", Lines: lines}},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())

	require.Equal(t, numLines, vc.TotalLines)

	// Set up for rendering only visible portion
	vc.SetWidth(100)
	viewportHeight := 50
	vc.SetVisibleRange(5000, viewportHeight) // Scroll to middle

	// Render should be fast since we only render visible + buffer
	content := vc.RenderVisible()
	contentLines := strings.Split(content, "\n")

	// Content should be total lines (with padding)
	require.Equal(t, numLines, len(contentLines))

	// Lines before visible range should be empty (padding)
	for i := 0; i < vc.VisibleStart; i++ {
		require.Empty(t, contentLines[i], "Line %d should be empty padding", i)
	}

	// Lines in visible range should have content
	for i := vc.VisibleStart; i < vc.VisibleEnd; i++ {
		require.NotEmpty(t, contentLines[i], "Line %d should have content", i)
	}

	// Lines after visible range should be empty (padding)
	for i := vc.VisibleEnd; i < numLines; i++ {
		require.Empty(t, contentLines[i], "Line %d should be empty padding", i)
	}
}

func TestVirtualContent_CacheHit(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,3 +1,3 @@",
				Lines: []DiffLine{
					{Type: LineHunkHeader},
					{Type: LineContext, Content: "line1", OldLineNum: 1, NewLineNum: 1},
					{Type: LineContext, Content: "line2", OldLineNum: 2, NewLineNum: 2},
				},
			},
		},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(80)
	vc.SetVisibleRange(0, 10)

	// First render populates cache
	_ = vc.RenderVisible()
	cacheSize1 := vc.RenderCache.Size()
	require.Greater(t, cacheSize1, 0)

	// Second render should use cache (no change in size)
	_ = vc.RenderVisible()
	cacheSize2 := vc.RenderCache.Size()
	require.Equal(t, cacheSize1, cacheSize2)
}

func TestCountTotalLines(t *testing.T) {
	tests := []struct {
		name     string
		files    []DiffFile
		expected int
	}{
		{
			name:     "empty files",
			files:    []DiffFile{},
			expected: 0,
		},
		{
			name: "single file with hunks",
			files: []DiffFile{
				{
					NewPath: "test.go",
					Hunks: []DiffHunk{
						{Lines: []DiffLine{{}, {}, {}}}, // 3 lines
						{Lines: []DiffLine{{}, {}}},     // 2 lines
					},
				},
			},
			expected: 5,
		},
		{
			name: "multiple files",
			files: []DiffFile{
				{
					NewPath: "file1.go",
					Hunks:   []DiffHunk{{Lines: []DiffLine{{}, {}}}}, // 2 lines
				},
				{
					NewPath: "file2.go",
					Hunks:   []DiffHunk{{Lines: []DiffLine{{}, {}, {}}}}, // 3 lines
				},
			},
			// For multiple files: 2 lines + header + separator + 3 lines + header + separator
			// = 2 + 2 + 3 + 2 = 9 (each file adds 2: header + separator)
			expected: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countTotalLines(tt.files)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestItoa(t *testing.T) {
	require.Equal(t, "0", itoa(0))
	require.Equal(t, "1", itoa(1))
	require.Equal(t, "42", itoa(42))
	require.Equal(t, "12345", itoa(12345))
	require.Equal(t, "-1", itoa(-1))
	require.Equal(t, "-42", itoa(-42))
}

func BenchmarkVirtualContent_RenderVisible(b *testing.B) {
	// Create a large diff
	numLines := 10000
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lines[i] = DiffLine{
			Type:       LineContext,
			Content:    "line content that is reasonably long to simulate real code",
			OldLineNum: i + 1,
			NewLineNum: i + 1,
		}
	}

	file := DiffFile{
		NewPath: "large.go",
		Hunks:   []DiffHunk{{Header: "@@ -1,10000 +1,10000 @@", Lines: lines}},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(120)
	vc.SetVisibleRange(5000, 50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = vc.RenderVisible()
	}
}

func BenchmarkVirtualContent_ScrollWithCache(b *testing.B) {
	// Simulate scrolling through a large diff
	numLines := 10000
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lines[i] = DiffLine{
			Type:       LineContext,
			Content:    "line content",
			OldLineNum: i + 1,
			NewLineNum: i + 1,
		}
	}

	file := DiffFile{
		NewPath: "large.go",
		Hunks:   []DiffHunk{{Header: "@@ -1,10000 +1,10000 @@", Lines: lines}},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(120)

	viewportHeight := 50

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate scrolling down by 1 line
		scrollPos := i % (numLines - viewportHeight)
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}
}

// === Enhanced LRU Cache Tests ===

func TestCacheMetrics_HitRate(t *testing.T) {
	tests := []struct {
		name     string
		hits     uint64
		misses   uint64
		expected float64
	}{
		{"no requests", 0, 0, 0},
		{"all hits", 10, 0, 100},
		{"all misses", 0, 10, 0},
		{"50/50", 5, 5, 50},
		{"80/20", 8, 2, 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := CacheMetrics{Hits: tt.hits, Misses: tt.misses}
			require.Equal(t, tt.expected, m.HitRate())
		})
	}
}

func TestRenderCache_Metrics(t *testing.T) {
	cache := NewRenderCache(100)

	// Initial state
	metrics := cache.GetMetrics()
	require.Equal(t, uint64(0), metrics.Hits)
	require.Equal(t, uint64(0), metrics.Misses)

	// Miss (get non-existent key)
	key := RenderCacheKey{LineIndex: 0, Width: 80}
	_, ok := cache.Get(key)
	require.False(t, ok)

	metrics = cache.GetMetrics()
	require.Equal(t, uint64(0), metrics.Hits)
	require.Equal(t, uint64(1), metrics.Misses)

	// Put and hit
	cache.Put(key, "test content")
	_, ok = cache.Get(key)
	require.True(t, ok)

	metrics = cache.GetMetrics()
	require.Equal(t, uint64(1), metrics.Hits)
	require.Equal(t, uint64(1), metrics.Misses)
	require.Equal(t, 50.0, metrics.HitRate())
}

func TestRenderCache_MemoryLimit(t *testing.T) {
	// Create a cache with 1KB limit
	cache := NewRenderCacheWithLimits(1000, 1024)

	// Each entry is roughly 50 + len(value) + len(FileHash) bytes
	// Create entries that should exceed the limit
	for i := 0; i < 20; i++ {
		key := RenderCacheKey{
			FileHash:  "test_file.go",
			LineIndex: i,
			Width:     80,
		}
		// Each value is 100 bytes, so ~162 bytes per entry
		value := strings.Repeat("x", 100)
		cache.Put(key, value)
	}

	// Should have evicted some entries due to size limit
	require.LessOrEqual(t, cache.ByteSize(), int64(1024))

	// Verify metrics recorded evictions
	metrics := cache.GetMetrics()
	require.Greater(t, metrics.Evictions, uint64(0))
}

func TestRenderCache_EnhancedKey(t *testing.T) {
	cache := NewRenderCache(100)

	// Same line index but different contexts should be different keys
	key1 := RenderCacheKey{
		FileHash:  "file1.go",
		HunkIndex: 0,
		LineIndex: 0,
		Width:     80,
		ViewMode:  ViewModeUnified,
		Theme:     "dark",
	}
	key2 := RenderCacheKey{
		FileHash:  "file2.go", // Different file
		HunkIndex: 0,
		LineIndex: 0,
		Width:     80,
		ViewMode:  ViewModeUnified,
		Theme:     "dark",
	}
	key3 := RenderCacheKey{
		FileHash:  "file1.go",
		HunkIndex: 0,
		LineIndex: 0,
		Width:     80,
		ViewMode:  ViewModeSideBySide, // Different view mode
		Theme:     "dark",
	}
	key4 := RenderCacheKey{
		FileHash:  "file1.go",
		HunkIndex: 0,
		LineIndex: 0,
		Width:     80,
		ViewMode:  ViewModeUnified,
		Theme:     "light", // Different theme
	}

	cache.Put(key1, "content1")
	cache.Put(key2, "content2")
	cache.Put(key3, "content3")
	cache.Put(key4, "content4")

	// All should be stored separately
	v1, ok := cache.Get(key1)
	require.True(t, ok)
	require.Equal(t, "content1", v1)

	v2, ok := cache.Get(key2)
	require.True(t, ok)
	require.Equal(t, "content2", v2)

	v3, ok := cache.Get(key3)
	require.True(t, ok)
	require.Equal(t, "content3", v3)

	v4, ok := cache.Get(key4)
	require.True(t, ok)
	require.Equal(t, "content4", v4)

	require.Equal(t, 4, cache.Size())
}

func TestRenderCache_SetViewMode(t *testing.T) {
	cache := NewRenderCache(100)

	// Initial mode is zero value (Unified)
	require.Equal(t, ViewModeUnified, cache.ViewMode())

	// Add some entries
	cache.Put(RenderCacheKey{LineIndex: 0, Width: 80}, "test")
	require.Equal(t, 1, cache.Size())

	// Setting same mode should not clear cache
	cleared := cache.SetViewMode(ViewModeUnified)
	require.False(t, cleared)
	require.Equal(t, 1, cache.Size())

	// Setting different mode should clear cache
	cleared = cache.SetViewMode(ViewModeSideBySide)
	require.True(t, cleared)
	require.Equal(t, 0, cache.Size())
	require.Equal(t, ViewModeSideBySide, cache.ViewMode())
}

func TestRenderCache_SetTheme(t *testing.T) {
	cache := NewRenderCache(100)

	// Initial theme is empty
	require.Equal(t, "", cache.Theme())

	// Add some entries
	cache.Put(RenderCacheKey{LineIndex: 0, Width: 80}, "test")
	require.Equal(t, 1, cache.Size())

	// Setting same theme should not clear cache
	cleared := cache.SetTheme("")
	require.False(t, cleared)
	require.Equal(t, 1, cache.Size())

	// Setting different theme should clear cache
	cleared = cache.SetTheme("dark")
	require.True(t, cleared)
	require.Equal(t, 0, cache.Size())
	require.Equal(t, "dark", cache.Theme())
}

func TestRenderCache_ResetMetrics(t *testing.T) {
	cache := NewRenderCache(100)

	// Generate some metrics
	cache.Get(RenderCacheKey{LineIndex: 0, Width: 80}) // miss
	cache.Put(RenderCacheKey{LineIndex: 0, Width: 80}, "test")
	cache.Get(RenderCacheKey{LineIndex: 0, Width: 80}) // hit

	metrics := cache.GetMetrics()
	require.Equal(t, uint64(1), metrics.Hits)
	require.Equal(t, uint64(1), metrics.Misses)

	// Reset and verify
	cache.ResetMetrics()
	metrics = cache.GetMetrics()
	require.Equal(t, uint64(0), metrics.Hits)
	require.Equal(t, uint64(0), metrics.Misses)
}

func TestVirtualContent_SetViewMode(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineContext, Content: "line", OldLineNum: 1, NewLineNum: 1},
				},
			},
		},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(80)
	vc.SetVisibleRange(0, 10)
	_ = vc.RenderVisible()

	require.Greater(t, vc.RenderCache.Size(), 0)

	// Change view mode
	cleared := vc.SetViewMode(ViewModeSideBySide)
	require.True(t, cleared)
	require.Equal(t, 0, vc.RenderCache.Size())
	require.Equal(t, ViewModeSideBySide, vc.ViewMode)
}

func TestVirtualContent_SetTheme(t *testing.T) {
	file := DiffFile{
		NewPath: "test.go",
		Hunks: []DiffHunk{
			{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []DiffLine{
					{Type: LineContext, Content: "line", OldLineNum: 1, NewLineNum: 1},
				},
			},
		},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(80)
	vc.SetVisibleRange(0, 10)
	_ = vc.RenderVisible()

	require.Greater(t, vc.RenderCache.Size(), 0)

	// Change theme
	cleared := vc.SetTheme("catppuccin")
	require.True(t, cleared)
	require.Equal(t, 0, vc.RenderCache.Size())
	require.Equal(t, "catppuccin", vc.Theme)
}

func TestVirtualContent_CacheHitRateTarget(t *testing.T) {
	// Create a reasonably sized diff (500 lines, just above threshold)
	numLines := 500
	lines := make([]DiffLine, numLines)
	for i := 0; i < numLines; i++ {
		lines[i] = DiffLine{
			Type:       LineContext,
			Content:    "line content here",
			OldLineNum: i + 1,
			NewLineNum: i + 1,
		}
	}

	file := DiffFile{
		NewPath: "test.go",
		Hunks:   []DiffHunk{{Header: "@@ -1,500 +1,500 @@", Lines: lines}},
	}

	vc := NewVirtualContentFromFile(file, DefaultVirtualContentConfig())
	vc.SetWidth(120)

	viewportHeight := 50

	// Simulate scrolling back and forth (which should hit cache)
	for scroll := 0; scroll < 200; scroll++ {
		pos := scroll % (numLines - viewportHeight)
		vc.SetVisibleRange(pos, viewportHeight)
		_ = vc.RenderVisible()
	}

	// After scroll simulation, check hit rate
	hitRate := vc.GetCacheHitRate()

	// Exit criteria: >80% cache hit rate during normal scrolling
	require.Greater(t, hitRate, 80.0,
		"Cache hit rate %.2f%% should be >80%% during normal scrolling", hitRate)
}

func TestVirtualContent_FileHashInLines(t *testing.T) {
	files := []DiffFile{
		{
			NewPath: "file1.go",
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,2 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader},
						{Type: LineAddition, Content: "new line", NewLineNum: 1},
					},
				},
			},
		},
		{
			NewPath: "file2.go",
			Hunks: []DiffHunk{
				{
					Header: "@@ -1,1 +1,1 @@",
					Lines: []DiffLine{
						{Type: LineHunkHeader},
						{Type: LineDeletion, Content: "old line", OldLineNum: 1},
					},
				},
			},
		},
	}

	vc := NewVirtualContent(files, DefaultVirtualContentConfig())

	// Verify file hashes are set
	for _, vl := range vc.Lines {
		if vl.IsFileHeader || vl.Type == LineContext && vl.Content == "" {
			// File headers and separators have special handling
			continue
		}
		require.NotEmpty(t, vl.FileHash, "FileHash should be set for line: %+v", vl)
	}

	// Verify file1 lines have file1.go hash
	require.Equal(t, "file1.go", vc.Lines[0].FileHash) // file header
	require.Equal(t, "file1.go", vc.Lines[1].FileHash) // hunk header
	require.Equal(t, "file1.go", vc.Lines[2].FileHash) // addition

	// Verify file2 lines have file2.go hash (after separator)
	// line 3 is separator, 4 is file2 header
	require.Equal(t, "file2.go", vc.Lines[4].FileHash) // file header
	require.Equal(t, "file2.go", vc.Lines[5].FileHash) // hunk header
	require.Equal(t, "file2.go", vc.Lines[6].FileHash) // deletion
}

func TestEntrySize(t *testing.T) {
	key := RenderCacheKey{
		FileHash:  "test.go",
		HunkIndex: 0,
		LineIndex: 0,
		Width:     80,
		ViewMode:  ViewModeUnified,
		Theme:     "dark",
	}
	value := "some rendered content"

	size := entrySize(key, value)

	// Size should include: value length + FileHash length + Theme length + overhead
	expectedMin := int64(len(value) + len(key.FileHash) + len(key.Theme))
	require.GreaterOrEqual(t, size, expectedMin)
}

func TestFileHashFromPath(t *testing.T) {
	tests := []struct {
		name     string
		newPath  string
		oldPath  string
		expected string
	}{
		{"new file", "file.go", "/dev/null", "file.go"},
		{"deleted file", "/dev/null", "file.go", "file.go"},
		{"modified file", "file.go", "file.go", "file.go"},
		{"renamed file", "new.go", "old.go", "new.go"},
		{"both null", "/dev/null", "/dev/null", "unknown"},
		{"empty paths", "", "", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fileHashFromPath(tt.newPath, tt.oldPath)
			require.Equal(t, tt.expected, result)
		})
	}
}

// TestRenderCache_Concurrent verifies thread safety of the RenderCache
// under concurrent access from multiple goroutines.
func TestRenderCache_Concurrent(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache(100)
	const numGoroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch goroutines that simultaneously Put and Get
	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				// Create keys unique to this goroutine and iteration
				key := RenderCacheKey{
					FileHash:  "test.go",
					LineIndex: goroutineID*opsPerGoroutine + i,
					Width:     80,
				}
				value := strings.Repeat("x", 50)

				// Mix of Put and Get operations
				cache.Put(key, value)
				_, _ = cache.Get(key)

				// Also access shared keys for contention
				sharedKey := RenderCacheKey{
					FileHash:  "shared.go",
					LineIndex: i % 10, // Only 10 shared keys
					Width:     80,
				}
				cache.Put(sharedKey, value)
				_, _ = cache.Get(sharedKey)
			}
		}(g)
	}

	wg.Wait()

	// Verify cache is in a consistent state
	size := cache.Size()
	require.GreaterOrEqual(t, size, 0)
	require.LessOrEqual(t, size, 100) // Should not exceed capacity

	metrics := cache.GetMetrics()
	// Total operations: each goroutine does 2 Gets per iteration
	// (one for unique key, one for shared key)
	expectedGets := uint64(numGoroutines * opsPerGoroutine * 2)
	actualGets := metrics.Hits + metrics.Misses
	require.Equal(t, expectedGets, actualGets,
		"Total Get operations should match expected")
}

// TestRenderCache_ConcurrentClear tests concurrent Clear operations
// alongside Get/Put to ensure no race conditions during cache clearing.
func TestRenderCache_ConcurrentClear(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache(50)
	const numWriters = 5
	const numReaders = 5
	const numClears = 2
	const opsPerWorker = 50

	var wg sync.WaitGroup
	wg.Add(numWriters + numReaders + numClears)

	// Writers
	for w := 0; w < numWriters; w++ {
		go func(writerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				key := RenderCacheKey{
					FileHash:  "file.go",
					LineIndex: writerID*opsPerWorker + i,
					Width:     80,
				}
				cache.Put(key, "content")
			}
		}(w)
	}

	// Readers
	for r := 0; r < numReaders; r++ {
		go func(readerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				key := RenderCacheKey{
					FileHash:  "file.go",
					LineIndex: i % 100,
					Width:     80,
				}
				_, _ = cache.Get(key)
			}
		}(r)
	}

	// Clearers
	for c := 0; c < numClears; c++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				cache.Clear()
			}
		}()
	}

	wg.Wait()

	// Cache should be in a consistent state (size >= 0, <= capacity)
	size := cache.Size()
	require.GreaterOrEqual(t, size, 0)
}

// TestRenderCache_ConcurrentMetrics verifies that metrics remain consistent
// under concurrent access - hits + misses should equal total Get calls.
func TestRenderCache_ConcurrentMetrics(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache(20) // Small cache to force evictions
	const numGoroutines = 8
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	var totalGets int64

	// Pre-populate some entries
	for i := 0; i < 10; i++ {
		key := RenderCacheKey{LineIndex: i, Width: 80}
		cache.Put(key, "initial")
	}
	cache.ResetMetrics() // Start fresh metrics

	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				// Access keys 0-29 (some exist, some don't)
				key := RenderCacheKey{LineIndex: i % 30, Width: 80}
				_, _ = cache.Get(key)
				atomic.AddInt64(&totalGets, 1)

				// Occasionally add new entries
				if i%5 == 0 {
					cache.Put(key, "updated")
				}
			}
		}()
	}

	wg.Wait()

	metrics := cache.GetMetrics()
	actualGets := metrics.Hits + metrics.Misses
	require.Equal(t, uint64(totalGets), actualGets,
		"Hits (%d) + Misses (%d) = %d should equal total Gets (%d)",
		metrics.Hits, metrics.Misses, actualGets, totalGets)
}

// TestRenderCache_ConcurrentViewModeChange tests concurrent view mode changes
// alongside regular cache operations.
func TestRenderCache_ConcurrentViewModeChange(t *testing.T) {
	t.Parallel()

	cache := NewRenderCache(50)
	const numWorkers = 4
	const opsPerWorker = 50

	var wg sync.WaitGroup
	wg.Add(numWorkers + 2) // workers + 2 mode changers

	// Regular workers doing Get/Put
	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				key := RenderCacheKey{
					FileHash:  "file.go",
					LineIndex: i,
					Width:     80,
				}
				cache.Put(key, "content")
				_, _ = cache.Get(key)
			}
		}(w)
	}

	// Mode changers
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			cache.SetViewMode(ViewModeUnified)
			cache.SetViewMode(ViewModeSideBySide)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			cache.SetTheme("dark")
			cache.SetTheme("light")
		}
	}()

	wg.Wait()

	// Verify cache is still functional
	key := RenderCacheKey{LineIndex: 0, Width: 80}
	cache.Put(key, "final")
	val, ok := cache.Get(key)
	require.True(t, ok)
	require.Equal(t, "final", val)
}

// TestRenderCache_ConcurrentByteSize tests that ByteSize remains consistent
// under concurrent modifications.
func TestRenderCache_ConcurrentByteSize(t *testing.T) {
	t.Parallel()

	cache := NewRenderCacheWithLimits(100, 10*1024) // 10KB limit
	const numGoroutines = 6
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := RenderCacheKey{
					FileHash:  "file.go",
					LineIndex: goroutineID*opsPerGoroutine + i,
					Width:     80,
				}
				// Variable size content
				content := strings.Repeat("x", 50+(i%100))
				cache.Put(key, content)

				// Read size while writes are happening
				_ = cache.ByteSize()
				_ = cache.Size()
			}
		}(g)
	}

	wg.Wait()

	// Verify final state is consistent
	byteSize := cache.ByteSize()
	require.GreaterOrEqual(t, byteSize, int64(0))
	require.LessOrEqual(t, byteSize, int64(10*1024)) // Should not exceed limit
}
