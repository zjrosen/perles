package diffviewer

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// generateSyntheticDiffFile creates a synthetic diff file with the specified number of lines.
// Returns a DiffFile with realistic content for benchmarking.
func generateSyntheticDiffFile(path string, lineCount int) DiffFile {
	hunks := make([]DiffHunk, 0)
	currentHunk := DiffHunk{
		OldStart: 1,
		NewStart: 1,
		Header:   "@@ -1,100 +1,100 @@ func example()",
		Lines:    make([]DiffLine, 0),
	}

	// Add hunk header line
	currentHunk.Lines = append(currentHunk.Lines, DiffLine{
		Type:    LineHunkHeader,
		Content: "@@ -1,100 +1,100 @@ func example()",
	})

	oldLineNum := 1
	newLineNum := 1

	for i := 0; i < lineCount; i++ {
		lineType := LineContext
		switch i % 5 {
		case 0:
			lineType = LineContext
		case 1:
			lineType = LineAddition
		case 2:
			lineType = LineDeletion
		case 3:
			lineType = LineContext
		case 4:
			lineType = LineAddition
		}

		// Generate realistic line content
		content := generateLineContent(i, lineType)

		line := DiffLine{
			Type:    lineType,
			Content: content,
		}

		switch lineType {
		case LineContext:
			line.OldLineNum = oldLineNum
			line.NewLineNum = newLineNum
			oldLineNum++
			newLineNum++
		case LineAddition:
			line.NewLineNum = newLineNum
			newLineNum++
		case LineDeletion:
			line.OldLineNum = oldLineNum
			oldLineNum++
		}

		currentHunk.Lines = append(currentHunk.Lines, line)

		// Start a new hunk every 100 lines
		if len(currentHunk.Lines) >= 100 {
			currentHunk.OldCount = oldLineNum - currentHunk.OldStart
			currentHunk.NewCount = newLineNum - currentHunk.NewStart
			hunks = append(hunks, currentHunk)

			currentHunk = DiffHunk{
				OldStart: oldLineNum,
				NewStart: newLineNum,
				Header:   fmt.Sprintf("@@ -%d,100 +%d,100 @@ func section_%d()", oldLineNum, newLineNum, len(hunks)),
				Lines:    make([]DiffLine, 0),
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    LineHunkHeader,
				Content: currentHunk.Header,
			})
		}
	}

	// Add remaining hunk if not empty
	if len(currentHunk.Lines) > 1 {
		currentHunk.OldCount = oldLineNum - currentHunk.OldStart
		currentHunk.NewCount = newLineNum - currentHunk.NewStart
		hunks = append(hunks, currentHunk)
	}

	// Calculate additions and deletions
	additions := 0
	deletions := 0
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case LineAddition:
				additions++
			case LineDeletion:
				deletions++
			}
		}
	}

	return DiffFile{
		OldPath:   path,
		NewPath:   path,
		Additions: additions,
		Deletions: deletions,
		Hunks:     hunks,
	}
}

// generateLineContent creates realistic line content for benchmarking.
func generateLineContent(lineNum int, lineType LineType) string {
	templates := []string{
		"func %s(ctx context.Context) error {",
		"	return fmt.Errorf(\"error: %w\", err)",
		"	log.Printf(\"Processing item %d\")",
		"	if err != nil {",
		"	}",
		"	for i := 0; i < len(items); i++ {",
		"	result := make(map[string]interface{})",
		"	defer cleanup()",
		"	// This is a comment explaining the code",
		"	data, err := json.Marshal(value)",
	}

	template := templates[lineNum%len(templates)]
	return fmt.Sprintf(template, lineNum)
}

// generateLargeDiff creates a large diff with the specified total line count.
func generateLargeDiff(totalLines int) []DiffFile {
	// Create multiple files that together have totalLines
	filesCount := (totalLines / 1000) + 1
	linesPerFile := totalLines / filesCount

	files := make([]DiffFile, filesCount)
	for i := 0; i < filesCount; i++ {
		path := fmt.Sprintf("pkg/module%d/file%d.go", i/10, i)
		files[i] = generateSyntheticDiffFile(path, linesPerFile)
	}

	return files
}

// ============================================================================
// Benchmark: Initial Render Time
// Target: <100ms for 10K lines
// ============================================================================

func BenchmarkInitialRender_1K(b *testing.B) {
	benchmarkInitialRender(b, 1000)
}

func BenchmarkInitialRender_10K(b *testing.B) {
	benchmarkInitialRender(b, 10000)
}

func BenchmarkInitialRender_50K(b *testing.B) {
	benchmarkInitialRender(b, 50000)
}

func benchmarkInitialRender(b *testing.B, lineCount int) {
	file := generateSyntheticDiffFile("test.go", lineCount)
	width := 120
	height := 50

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = renderDiffContentWithWordDiff(file, nil, width, height)
	}
}

// ============================================================================
// Benchmark: Virtual Scrolling Render Time
// Target: <16ms for 60fps smooth scrolling
// ============================================================================

func BenchmarkVirtualScrollRender_1K(b *testing.B) {
	benchmarkVirtualScrollRender(b, 1000)
}

func BenchmarkVirtualScrollRender_10K(b *testing.B) {
	benchmarkVirtualScrollRender(b, 10000)
}

func BenchmarkVirtualScrollRender_50K(b *testing.B) {
	benchmarkVirtualScrollRender(b, 50000)
}

func benchmarkVirtualScrollRender(b *testing.B, lineCount int) {
	file := generateSyntheticDiffFile("test.go", lineCount)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	viewportHeight := 50
	totalLines := vc.TotalLines

	b.ReportAllocs()
	b.ResetTimer()

	// Simulate scrolling through the content
	for i := 0; i < b.N; i++ {
		scrollPos := (i * 10) % max(totalLines-viewportHeight, 1)
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}
}

// ============================================================================
// Benchmark: Scroll Latency (single frame render)
// Target: <16ms for 60fps
// ============================================================================

func BenchmarkScrollLatency_10K(b *testing.B) {
	benchmarkScrollLatency(b, 10000)
}

func benchmarkScrollLatency(b *testing.B, lineCount int) {
	file := generateSyntheticDiffFile("test.go", lineCount)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	viewportHeight := 50

	// Warm up the cache with a few renders
	for i := 0; i < 10; i++ {
		vc.SetVisibleRange(i*10, viewportHeight)
		_ = vc.RenderVisible()
	}

	b.ReportAllocs()
	b.ResetTimer()

	// Measure single scroll operations (simulating frame-by-frame scroll)
	for i := 0; i < b.N; i++ {
		// Small incremental scroll (1 line at a time)
		scrollPos := (i % max(vc.TotalLines-viewportHeight, 1))
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}
}

// ============================================================================
// Benchmark: Cache Performance
// Target: >80% hit rate after warm-up
// ============================================================================

func BenchmarkCacheHitRate_10K(b *testing.B) {
	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	viewportHeight := 50
	totalLines := vc.TotalLines

	// Simulate realistic scrolling pattern (back and forth)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Scroll forward
		scrollPos := (i * 5) % max(totalLines-viewportHeight, 1)
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()

		// Scroll back slightly (should hit cache)
		if scrollPos > 10 {
			vc.SetVisibleRange(scrollPos-10, viewportHeight)
			_ = vc.RenderVisible()
		}
	}

	// Report cache metrics
	metrics := vc.GetCacheMetrics()
	b.ReportMetric(metrics.HitRate(), "hit_rate_%")
	b.ReportMetric(float64(metrics.Hits), "cache_hits")
	b.ReportMetric(float64(metrics.Misses), "cache_misses")
}

func BenchmarkCacheWithThemeChange(b *testing.B) {
	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	viewportHeight := 50
	themes := []string{"dark", "light", "mocha"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Change theme periodically
		if i%100 == 0 {
			vc.SetTheme(themes[i/100%len(themes)])
		}

		scrollPos := (i * 3) % max(vc.TotalLines-viewportHeight, 1)
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}
}

// ============================================================================
// Benchmark: Memory Usage
// Target: <50MB for 10K lines
// ============================================================================

func BenchmarkMemoryUsage_10K(b *testing.B) {
	benchmarkMemoryUsage(b, 10000)
}

func benchmarkMemoryUsage(b *testing.B, lineCount int) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		runtime.GC()
		runtime.GC()

		var memBefore runtime.MemStats
		runtime.ReadMemStats(&memBefore)
		baselineAlloc := memBefore.TotalAlloc

		file := generateSyntheticDiffFile("test.go", lineCount)
		config := DefaultVirtualContentConfig()
		vc := NewVirtualContentFromFile(file, config)
		vc.SetWidth(120)

		// Simulate full scroll through content
		viewportHeight := 50
		for scrollPos := 0; scrollPos < vc.TotalLines; scrollPos += viewportHeight {
			vc.SetVisibleRange(scrollPos, viewportHeight)
			_ = vc.RenderVisible()
		}

		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		memUsedBytes := memAfter.TotalAlloc - baselineAlloc
		memUsedMB := float64(memUsedBytes) / (1024 * 1024)
		b.ReportMetric(memUsedMB, "MB_allocated")
	}
}

// ============================================================================
// Benchmark: File Tree Render
// ============================================================================

func BenchmarkFileTreeRender_100Files(b *testing.B) {
	benchmarkFileTreeRender(b, 100)
}

func BenchmarkFileTreeRender_1000Files(b *testing.B) {
	benchmarkFileTreeRender(b, 1000)
}

func benchmarkFileTreeRender(b *testing.B, fileCount int) {
	files := make([]DiffFile, fileCount)
	for i := 0; i < fileCount; i++ {
		files[i] = DiffFile{
			OldPath:   fmt.Sprintf("pkg/module%d/file%d.go", i/10, i),
			NewPath:   fmt.Sprintf("pkg/module%d/file%d.go", i/10, i),
			Additions: i % 100,
			Deletions: i % 50,
		}
	}

	tree := NewFileTree(files)
	width := 50
	height := 30

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		selectedIdx := i % fileCount
		scrollTop := max(0, selectedIdx-height/2)
		_ = renderFileTree(tree, selectedIdx, scrollTop, width, height, true)
	}
}

// ============================================================================
// Benchmark: Render Cache Operations
// ============================================================================

func BenchmarkRenderCache_Put(b *testing.B) {
	cache := NewRenderCache(1000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := RenderCacheKey{
			FileHash:  "test.go",
			HunkIndex: i % 100,
			LineIndex: i,
			Width:     120,
			ViewMode:  ViewModeUnified,
			Theme:     "dark",
		}
		value := strings.Repeat("x", 100)
		cache.Put(key, value)
	}
}

func BenchmarkRenderCache_Get(b *testing.B) {
	cache := NewRenderCache(1000)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := RenderCacheKey{
			FileHash:  "test.go",
			HunkIndex: i % 100,
			LineIndex: i,
			Width:     120,
			ViewMode:  ViewModeUnified,
			Theme:     "dark",
		}
		cache.Put(key, strings.Repeat("x", 100))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := RenderCacheKey{
			FileHash:  "test.go",
			HunkIndex: i % 100,
			LineIndex: i % 1000,
			Width:     120,
			ViewMode:  ViewModeUnified,
			Theme:     "dark",
		}
		_, _ = cache.Get(key)
	}
}

// ============================================================================
// Test: Performance Thresholds (Fails CI if exceeded)
// ============================================================================

// TestPerformanceThresholds tests that performance meets documented requirements.
// These tests will fail if performance regresses beyond acceptable thresholds.
func TestPerformanceThresholds_InitialRender(t *testing.T) {
	// Target: <150ms for 10K lines initial render (with CI variance headroom)
	file := generateSyntheticDiffFile("test.go", 10000)
	width := 120
	height := 0 // Render all

	result := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = renderDiffContentWithWordDiff(file, nil, width, height)
		}
	})

	// Calculate average time per operation in milliseconds
	avgMs := float64(result.T.Nanoseconds()) / float64(result.N) / 1e6

	// Log the actual time for debugging
	t.Logf("Initial render time for 10K lines: %.2fms (target: <150ms)", avgMs)

	// Fail if too slow (with headroom for CI variance)
	require.Less(t, avgMs, 150.0, "Initial render for 10K lines should be <150ms, got %.2fms", avgMs)
}

func TestPerformanceThresholds_ScrollLatency(t *testing.T) {
	// Target: <16ms for 60fps scrolling
	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)
	viewportHeight := 50

	// Warm up cache
	for i := 0; i < 10; i++ {
		vc.SetVisibleRange(i*10, viewportHeight)
		_ = vc.RenderVisible()
	}

	result := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			scrollPos := i % max(vc.TotalLines-viewportHeight, 1)
			vc.SetVisibleRange(scrollPos, viewportHeight)
			_ = vc.RenderVisible()
		}
	})

	avgMs := float64(result.T.Nanoseconds()) / float64(result.N) / 1e6

	t.Logf("Scroll latency for 10K lines: %.2fms (target: <16ms for 60fps)", avgMs)

	require.Less(t, avgMs, 16.0, "Scroll latency should be <16ms for 60fps, got %.2fms", avgMs)
}

func TestPerformanceThresholds_MemoryUsage(t *testing.T) {
	// Target: <50MB for 10K lines (retained heap memory)
	// Force GC and get baseline
	runtime.GC()
	runtime.GC() // Double GC for more reliable baseline

	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	baselineHeap := memBefore.HeapAlloc

	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	// Simulate full scroll through content to populate cache
	viewportHeight := 50
	for scrollPos := 0; scrollPos < vc.TotalLines; scrollPos += viewportHeight {
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}

	// Force GC before measuring to get retained memory
	runtime.GC()
	runtime.GC()

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	// Measure retained heap memory (what's actually held in memory)
	var heapUsedMB float64
	if memAfter.HeapAlloc > baselineHeap {
		heapUsedMB = float64(memAfter.HeapAlloc-baselineHeap) / (1024 * 1024)
	} else {
		heapUsedMB = float64(memAfter.HeapAlloc) / (1024 * 1024)
	}

	t.Logf("Retained heap memory for 10K lines: %.2fMB (target: <50MB)", heapUsedMB)
	t.Logf("Cache byte size: %d bytes", vc.RenderCache.ByteSize())

	// Keep vc alive until after measurement
	_ = vc.TotalLines

	require.Less(t, heapUsedMB, 50.0, "Memory usage for 10K lines should be <50MB, got %.2fMB", heapUsedMB)
}

func TestPerformanceThresholds_CacheHitRate(t *testing.T) {
	// Target: >80% cache hit rate
	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	viewportHeight := 50
	totalLines := vc.TotalLines

	// Simulate realistic scrolling pattern - scroll through then back
	for scrollPos := 0; scrollPos < min(totalLines-viewportHeight, 500); scrollPos += 10 {
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}

	// Scroll back (should hit cache heavily)
	for scrollPos := min(totalLines-viewportHeight, 500) - 10; scrollPos >= 0; scrollPos -= 10 {
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}

	metrics := vc.GetCacheMetrics()
	hitRate := metrics.HitRate()

	t.Logf("Cache hit rate: %.1f%% (target: >80%%)", hitRate)
	t.Logf("Cache stats: hits=%d, misses=%d, evictions=%d", metrics.Hits, metrics.Misses, metrics.Evictions)

	require.Greater(t, hitRate, 80.0, "Cache hit rate should be >80%%, got %.1f%%", hitRate)
}

// ============================================================================
// Benchmark: Side-by-Side Virtual Scrolling
// Tests the new side-by-side virtual scrolling implementation
// ============================================================================

func BenchmarkSideBySideVirtualScroll_10K(b *testing.B) {
	benchmarkSideBySideVirtualScroll(b, 10000)
}

func BenchmarkSideBySideVirtualScroll_50K(b *testing.B) {
	benchmarkSideBySideVirtualScroll(b, 50000)
}

func benchmarkSideBySideVirtualScroll(b *testing.B, lineCount int) {
	file := generateSyntheticDiffFile("test.go", lineCount)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	// Switch to side-by-side mode
	vc.SetViewMode(ViewModeSideBySide)

	viewportHeight := 50

	b.ReportAllocs()
	b.ResetTimer()

	// Simulate scrolling through the content in side-by-side mode
	for i := 0; i < b.N; i++ {
		scrollPos := (i * 10) % max(vc.TotalLines-viewportHeight, 1)
		vc.SetVisibleRange(scrollPos, viewportHeight)
		_ = vc.RenderVisible()
	}
}

// BenchmarkSideBySideVsUnified compares rendering performance between view modes
func BenchmarkSideBySideVsUnified_10K(b *testing.B) {
	file := generateSyntheticDiffFile("test.go", 10000)
	config := DefaultVirtualContentConfig()
	viewportHeight := 50

	b.Run("Unified", func(b *testing.B) {
		vc := NewVirtualContentFromFile(file, config)
		vc.SetWidth(120)
		vc.SetViewMode(ViewModeUnified)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			scrollPos := (i * 10) % max(vc.TotalLines-viewportHeight, 1)
			vc.SetVisibleRange(scrollPos, viewportHeight)
			_ = vc.RenderVisible()
		}
	})

	b.Run("SideBySide", func(b *testing.B) {
		vc := NewVirtualContentFromFile(file, config)
		vc.SetWidth(120)
		vc.SetViewMode(ViewModeSideBySide)

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			scrollPos := (i * 10) % max(vc.TotalLines-viewportHeight, 1)
			vc.SetVisibleRange(scrollPos, viewportHeight)
			_ = vc.RenderVisible()
		}
	})
}

// TestSideBySideVirtualScrollingCorrectness verifies side-by-side output contains expected content
func TestSideBySideVirtualScrollingCorrectness(t *testing.T) {
	file := generateSyntheticDiffFile("test.go", 100)
	config := DefaultVirtualContentConfig()
	vc := NewVirtualContentFromFile(file, config)
	vc.SetWidth(120)

	// Verify side-by-side lines were created
	require.NotEmpty(t, vc.SideBySideLines, "should have pre-computed side-by-side lines")

	// Switch to side-by-side mode
	vc.SetViewMode(ViewModeSideBySide)
	require.Equal(t, vc.SideBySideTotalLines, vc.TotalLines, "TotalLines should match SideBySideTotalLines")

	// Render some visible lines
	vc.SetVisibleRange(0, 20)
	output := vc.RenderVisible()

	// Output should contain the side-by-side separator
	require.Contains(t, output, "│", "side-by-side output should contain separator")

	// Should have multiple lines
	lines := strings.Split(output, "\n")
	require.Greater(t, len(lines), 10, "should render at least 10 lines")
}
