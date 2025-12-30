package chainart

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildChainArt(t *testing.T) {
	art := BuildChainArt()

	// Should contain 6 lines (broken link height)
	lines := strings.Split(art, "\n")
	require.Equal(t, 6, len(lines), "expected 6 lines")

	// Should contain broken link characters
	require.True(t, strings.Contains(art, "\\│/"), "expected broken link crack characters")
}

func TestBuildIntactChainArt(t *testing.T) {
	art := BuildIntactChainArt()

	// Should contain 4 lines (all links same height)
	lines := strings.Split(art, "\n")
	require.Equal(t, 4, len(lines), "expected 4 lines")

	// Should NOT contain broken link characters
	require.False(t, strings.Contains(art, "\\│/"), "intact chain should not have broken link crack characters")
	require.False(t, strings.Contains(art, "/│\\"), "intact chain should not have broken link crack characters")

	// Should contain standard link box characters
	require.True(t, strings.Contains(art, "╔═══════╗"), "expected standard link box top")
	require.True(t, strings.Contains(art, "╚═══════╝"), "expected standard link box bottom")

	// Should contain connectors
	require.True(t, strings.Contains(art, "═══"), "expected connector characters")
}

func TestBuildIntactChainArt_FiveLinkStructure(t *testing.T) {
	art := BuildIntactChainArt()

	// Count link boundaries - should have 5 top corners and 5 bottom corners
	topCornerCount := strings.Count(art, "╔═══════╗")
	bottomCornerCount := strings.Count(art, "╚═══════╝")

	require.Equal(t, 5, topCornerCount, "expected 5 link tops (╔═══════╗)")
	require.Equal(t, 5, bottomCornerCount, "expected 5 link bottoms (╚═══════╝)")
}

func TestBuildIntactChainArt_HasConnectors(t *testing.T) {
	art := BuildIntactChainArt()
	lines := strings.Split(art, "\n")

	// Lines 1 and 2 (0-indexed) should have connectors between links
	// Each connector is "═══" and there are 4 connectors (between 5 links)
	for i, line := range lines {
		if i == 1 || i == 2 {
			// These lines should have connectors
			connectorCount := strings.Count(line, "═══")
			// Connectors appear between links, but "═══" also appears in box tops/bottoms
			// On middle lines, we expect connector pieces
			require.GreaterOrEqual(t, connectorCount, 4, "line %d expected at least 4 connector segments", i)
		}
	}
}

func TestBuildIntactChainArt_ConsistentLineWidths(t *testing.T) {
	art := BuildIntactChainArt()
	lines := strings.Split(art, "\n")

	// All lines should have consistent visual width for proper alignment
	require.GreaterOrEqual(t, len(lines), 4, "expected at least 4 lines")

	// The first and last lines (top/bottom of boxes) should be similar width
	// This ensures proper visual alignment
	firstLineLen := len([]rune(lines[0]))
	lastLineLen := len([]rune(lines[3]))

	require.Equal(t, firstLineLen, lastLineLen, "first line width != last line width, alignment issue")
}

func TestBuildProgressChainArt_NoFailure_AllPending(t *testing.T) {
	art := BuildProgressChainArt(0, -1)
	lines := strings.Split(art, "\n")

	// Should be 4 lines (intact chain - no failure)
	require.Equal(t, 4, len(lines), "expected 4 lines for intact chain")

	// Should have 5 link tops
	topCount := strings.Count(art, "╔═══════╗")
	require.Equal(t, 5, topCount, "expected 5 link tops")
}

func TestBuildProgressChainArt_NoFailure_AllComplete(t *testing.T) {
	art := BuildProgressChainArt(5, -1)
	lines := strings.Split(art, "\n")

	// Should be 4 lines (intact chain)
	require.Equal(t, 4, len(lines), "expected 4 lines for intact chain")

	// Should have 5 link tops
	topCount := strings.Count(art, "╔═══════╗")
	require.Equal(t, 5, topCount, "expected 5 link tops")
}

func TestBuildProgressChainArt_FailedAtFirstPhase(t *testing.T) {
	art := BuildProgressChainArt(0, 0) // Failed at first phase
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link which is taller)
	require.Equal(t, 6, len(lines), "expected 6 lines with broken link")

	// Should contain broken link characters
	require.True(t, strings.Contains(art, "\\│/"), "expected broken link crack characters")

	// Should have 4 regular link tops (broken link has different structure)
	topCount := strings.Count(art, "╔═══════╗")
	require.Equal(t, 4, topCount, "expected 4 regular link tops (1 broken)")
}

func TestBuildProgressChainArt_FailedAtMiddlePhase(t *testing.T) {
	art := BuildProgressChainArt(2, 2) // Failed at phase 2 (SpawningWorkers)
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link)
	require.Equal(t, 6, len(lines), "expected 6 lines with broken link")

	// Should contain broken link characters
	require.True(t, strings.Contains(art, "\\│/"), "expected broken link crack characters")
}

func TestBuildProgressChainArt_FailedAtLastPhase(t *testing.T) {
	art := BuildProgressChainArt(3, 3) // Failed at phase 3 (WorkersReady)
	lines := strings.Split(art, "\n")

	// Should be 6 lines (has broken link)
	require.Equal(t, 6, len(lines), "expected 6 lines with broken link")

	// Should contain broken link characters
	require.True(t, strings.Contains(art, "\\│/"), "expected broken link crack characters")
}
