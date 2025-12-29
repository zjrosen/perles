package issuebadge

import (
	"strings"
	"testing"

	"github.com/zjrosen/perles/internal/beads"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
)

// stripANSI removes ANSI escape codes for easier testing of content
func stripANSI(s string) string {
	// Simple approach: remove escape sequences
	result := s
	for strings.Contains(result, "\x1b[") {
		start := strings.Index(result, "\x1b[")
		end := start + 2
		for end < len(result) && result[end] != 'm' {
			end++
		}
		if end < len(result) {
			result = result[:start] + result[end+1:]
		} else {
			break
		}
	}
	return result
}

func TestRenderBadge_IssueTypes(t *testing.T) {
	tests := []struct {
		name         string
		issueType    beads.IssueType
		wantContains string
	}{
		{
			name:         "epic",
			issueType:    beads.TypeEpic,
			wantContains: "[E]",
		},
		{
			name:         "task",
			issueType:    beads.TypeTask,
			wantContains: "[T]",
		},
		{
			name:         "feature",
			issueType:    beads.TypeFeature,
			wantContains: "[F]",
		},
		{
			name:         "bug",
			issueType:    beads.TypeBug,
			wantContains: "[B]",
		},
		{
			name:         "chore",
			issueType:    beads.TypeChore,
			wantContains: "[C]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := beads.Issue{
				ID:        "test-123",
				Type:      tt.issueType,
				Priority:  beads.PriorityMedium,
				TitleText: "Test issue",
			}

			got := RenderBadge(issue)
			stripped := stripANSI(got)

			if !strings.Contains(stripped, tt.wantContains) {
				t.Errorf("RenderBadge() = %q (stripped: %q), want to contain %q", got, stripped, tt.wantContains)
			}
		})
	}
}

func TestRenderBadge_Priorities(t *testing.T) {
	tests := []struct {
		name         string
		priority     beads.Priority
		wantContains string
	}{
		{
			name:         "P0 critical",
			priority:     beads.PriorityCritical,
			wantContains: "[P0]",
		},
		{
			name:         "P1 high",
			priority:     beads.PriorityHigh,
			wantContains: "[P1]",
		},
		{
			name:         "P2 medium",
			priority:     beads.PriorityMedium,
			wantContains: "[P2]",
		},
		{
			name:         "P3 low",
			priority:     beads.PriorityLow,
			wantContains: "[P3]",
		},
		{
			name:         "P4 backlog",
			priority:     beads.PriorityBacklog,
			wantContains: "[P4]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := beads.Issue{
				ID:        "test-123",
				Type:      beads.TypeTask,
				Priority:  tt.priority,
				TitleText: "Test issue",
			}

			got := RenderBadge(issue)
			stripped := stripANSI(got)

			if !strings.Contains(stripped, tt.wantContains) {
				t.Errorf("RenderBadge() = %q (stripped: %q), want to contain %q", got, stripped, tt.wantContains)
			}
		})
	}
}

func TestRenderBadge_Format(t *testing.T) {
	issue := beads.Issue{
		ID:        "abc-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
	}

	got := RenderBadge(issue)
	stripped := stripANSI(got)

	// Should contain type, priority, and ID in that order
	if !strings.Contains(stripped, "[T][P2][abc-123]") {
		t.Errorf("RenderBadge() = %q (stripped: %q), want format [T][P2][abc-123]", got, stripped)
	}

	// Should NOT contain the title
	if strings.Contains(stripped, "Test issue") {
		t.Errorf("RenderBadge() should not contain title, got: %q", stripped)
	}
}

func TestRender_IncludesTitle(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "My test title",
	}

	got := Render(issue, Config{})
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "My test title") {
		t.Errorf("Render() = %q (stripped: %q), want to contain title", got, stripped)
	}
}

func TestRender_SelectionIndicator(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
	}

	tests := []struct {
		name          string
		cfg           Config
		wantPrefix    string
		notWantPrefix string
	}{
		{
			name:       "no selection indicator",
			cfg:        Config{ShowSelection: false},
			wantPrefix: "[T]", // Starts directly with badge
		},
		{
			name:       "selection indicator - not selected",
			cfg:        Config{ShowSelection: true, Selected: false},
			wantPrefix: " [T]", // Space + badge
		},
		{
			name:       "selection indicator - selected",
			cfg:        Config{ShowSelection: true, Selected: true},
			wantPrefix: ">[T]", // > + badge
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(issue, tt.cfg)
			stripped := stripANSI(got)

			if !strings.HasPrefix(stripped, tt.wantPrefix) {
				t.Errorf("Render() = %q (stripped: %q), want prefix %q", got, stripped, tt.wantPrefix)
			}
		})
	}
}

func TestRender_TitleTruncation(t *testing.T) {
	issue := beads.Issue{
		ID:        "id",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "This is a very long title that should be truncated",
	}

	// Calculate approximate badge width: [T][P2][id] = 3+4+4 = 11 chars
	// With selection (space) = 12 chars
	// Plus space before title = 13 chars
	// MaxWidth of 30 leaves ~17 chars for title

	got := Render(issue, Config{
		ShowSelection: true,
		Selected:      false,
		MaxWidth:      30,
	})

	// The rendered output should be at most 30 characters wide
	width := lipgloss.Width(got)
	if width > 30 {
		t.Errorf("Render() width = %d, want <= 30", width)
	}

	// Should contain ellipsis since title was truncated
	stripped := stripANSI(got)
	if !strings.Contains(stripped, "...") {
		t.Errorf("Render() = %q, expected truncation with ellipsis", stripped)
	}
}

func TestRender_NoTruncationWhenFits(t *testing.T) {
	issue := beads.Issue{
		ID:        "id",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Short",
	}

	got := Render(issue, Config{
		MaxWidth: 100, // Plenty of space
	})

	stripped := stripANSI(got)
	if !strings.Contains(stripped, "Short") {
		t.Errorf("Render() = %q, want to contain full title 'Short'", stripped)
	}

	// Should NOT contain ellipsis
	if strings.Contains(stripped, "...") {
		t.Errorf("Render() = %q, should not truncate short title", stripped)
	}
}

func TestRender_ZeroMaxWidthNoTruncation(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "This is a very long title that should not be truncated when MaxWidth is 0",
	}

	got := Render(issue, Config{
		MaxWidth: 0, // No limit
	})

	stripped := stripANSI(got)
	if !strings.Contains(stripped, "This is a very long title that should not be truncated when MaxWidth is 0") {
		t.Errorf("Render() = %q, want to contain full title", stripped)
	}
}

// TestRenderBadge_Golden tests badge rendering with ANSI styles.
// Run with -update flag to update golden files: go test ./internal/ui/shared/issuebadge -update
func TestRenderBadge_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:        "abc-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
	}

	got := RenderBadge(issue)
	teatest.RequireEqualOutput(t, []byte(got))
}

// TestRenderBadge_Epic_Golden tests epic badge rendering.
func TestRenderBadge_Epic_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:       "epic-42",
		Type:     beads.TypeEpic,
		Priority: beads.PriorityCritical,
	}

	got := RenderBadge(issue)
	teatest.RequireEqualOutput(t, []byte(got))
}

// TestRenderBadge_Bug_Golden tests bug badge rendering.
func TestRenderBadge_Bug_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:       "bug-99",
		Type:     beads.TypeBug,
		Priority: beads.PriorityHigh,
	}

	got := RenderBadge(issue)
	teatest.RequireEqualOutput(t, []byte(got))
}

// TestRender_Selected_Golden tests full render with selection indicator.
func TestRender_Selected_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:        "task-1",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Implement feature X",
	}

	got := Render(issue, Config{
		ShowSelection: true,
		Selected:      true,
	})
	teatest.RequireEqualOutput(t, []byte(got))
}

// TestRender_NotSelected_Golden tests full render without selection.
func TestRender_NotSelected_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:        "task-1",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Implement feature X",
	}

	got := Render(issue, Config{
		ShowSelection: true,
		Selected:      false,
	})
	teatest.RequireEqualOutput(t, []byte(got))
}

// TestRender_Truncated_Golden tests title truncation with MaxWidth.
func TestRender_Truncated_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:        "id",
		Type:      beads.TypeFeature,
		Priority:  beads.PriorityLow,
		TitleText: "This is a very long title that should be truncated",
	}

	got := Render(issue, Config{
		ShowSelection: true,
		Selected:      true,
		MaxWidth:      40,
	})
	teatest.RequireEqualOutput(t, []byte(got))
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}

// TestRenderBadge_PinnedNil verifies no pin emoji when Pinned is nil (default).
func TestRenderBadge_PinnedNil(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-123",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
		Pinned:    nil, // explicitly nil
	}

	got := RenderBadge(issue)
	stripped := stripANSI(got)

	if strings.Contains(stripped, "📌") {
		t.Errorf("RenderBadge() with Pinned=nil should not contain pin emoji, got: %q", stripped)
	}

	// Should still contain the expected badge format
	if !strings.Contains(stripped, "[T][P2][test-123]") {
		t.Errorf("RenderBadge() = %q, want format [T][P2][test-123]", stripped)
	}
}

// TestRenderBadge_PinnedFalse verifies no pin emoji when Pinned is false.
func TestRenderBadge_PinnedFalse(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-456",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
		Pinned:    boolPtr(false),
	}

	got := RenderBadge(issue)
	stripped := stripANSI(got)

	if strings.Contains(stripped, "📌") {
		t.Errorf("RenderBadge() with Pinned=false should not contain pin emoji, got: %q", stripped)
	}

	// Should still contain the expected badge format
	if !strings.Contains(stripped, "[T][P2][test-456]") {
		t.Errorf("RenderBadge() = %q, want format [T][P2][test-456]", stripped)
	}
}

// TestRenderBadge_PinnedTrue verifies pin emoji appears when Pinned is true.
func TestRenderBadge_PinnedTrue(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-789",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityMedium,
		TitleText: "Test issue",
		Pinned:    boolPtr(true),
	}

	got := RenderBadge(issue)
	stripped := stripANSI(got)

	if !strings.Contains(stripped, "📌") {
		t.Errorf("RenderBadge() with Pinned=true should contain pin emoji, got: %q", stripped)
	}

	// Pin should come before the type indicator
	if !strings.HasPrefix(stripped, "📌[T]") {
		t.Errorf("RenderBadge() = %q, pin should precede type indicator [T]", stripped)
	}

	// Should contain full badge format with pin
	if !strings.Contains(stripped, "📌[T][P2][test-789]") {
		t.Errorf("RenderBadge() = %q, want format 📌[T][P2][test-789]", stripped)
	}
}

// TestRenderBadge_Pinned_Golden captures exact ANSI output for pinned badge.
func TestRenderBadge_Pinned_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:       "pinned-1",
		Type:     beads.TypeTask,
		Priority: beads.PriorityHigh,
		Pinned:   boolPtr(true),
	}

	got := RenderBadge(issue)
	teatest.RequireEqualOutput(t, []byte(got))
}
