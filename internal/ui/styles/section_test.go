package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestRenderFormSection(t *testing.T) {
	// Use a consistent focus color for tests
	focusColor := lipgloss.Color("#54A0FF")

	tests := []struct {
		name           string
		content        []string
		title          string
		hint           string
		width          int
		focused        bool
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:    "basic section with title",
			content: []string{"  Content line"},
			title:   "Name",
			hint:    "",
			width:   30,
			focused: false,
			wantContains: []string{
				"╭─ Name",
				"│",
				"Content line",
				"╰",
			},
		},
		{
			name:    "section with title and hint",
			content: []string{"  Input here"},
			title:   "Query",
			hint:    "required",
			width:   40,
			focused: false,
			wantContains: []string{
				"╭─ Query",
				"(required)",
				"│",
				"Input here",
				"╰",
			},
		},
		{
			name:    "empty title renders plain border",
			content: []string{"Content"},
			title:   "",
			hint:    "",
			width:   20,
			focused: false,
			wantContains: []string{
				"╭",
				"─",
				"╮",
				"│",
				"Content",
				"╰",
				"╯",
			},
			wantNotContain: []string{
				"╭─ ", // No title formatting
			},
		},
		{
			name:    "multiple content lines",
			content: []string{"Line 1", "Line 2", "Line 3"},
			title:   "Items",
			hint:    "",
			width:   25,
			focused: false,
			wantContains: []string{
				"Line 1",
				"Line 2",
				"Line 3",
			},
		},
		{
			name:    "focused section",
			content: []string{"Focused content"},
			title:   "Focus",
			hint:    "",
			width:   30,
			focused: true,
			wantContains: []string{
				"╭─ Focus",
				"│",
				"Focused content",
				"╰",
			},
		},
		{
			name:    "narrow width handles gracefully",
			content: []string{"X"},
			title:   "T",
			hint:    "",
			width:   5,
			focused: false,
			wantContains: []string{
				"╭",
				"╮",
				"│",
				"X",
				"╰",
				"╯",
			},
		},
		{
			name:    "minimum width",
			content: []string{"A"},
			title:   "",
			hint:    "",
			width:   3,
			focused: false,
			wantContains: []string{
				"╭",
				"╮",
				"│",
				"╰",
				"╯",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderFormSection(tt.content, tt.title, tt.hint, tt.width, tt.focused, focusColor)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("RenderFormSection() missing expected content %q\nGot:\n%s", want, result)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(result, notWant) {
					t.Errorf("RenderFormSection() contains unexpected %q\nGot:\n%s", notWant, result)
				}
			}
		})
	}
}

func TestRenderFormSection_FocusChangesColor(t *testing.T) {
	// Force ANSI color output in test environment
	lipgloss.SetColorProfile(termenv.ANSI256)

	content := []string{"Content"}
	title := "Test"
	focusColor := lipgloss.Color("#54A0FF")

	unfocused := RenderFormSection(content, title, "", 30, false, focusColor)
	focused := RenderFormSection(content, title, "", 30, true, focusColor)

	// Both should contain the same structural elements
	for _, want := range []string{"╭", "╮", "│", "╰", "╯", "Content", "Test"} {
		if !strings.Contains(unfocused, want) {
			t.Errorf("Unfocused section missing %q", want)
		}
		if !strings.Contains(focused, want) {
			t.Errorf("Focused section missing %q", want)
		}
	}

	// The outputs should be different (different ANSI color codes)
	if unfocused == focused {
		t.Error("Focused and unfocused sections should have different ANSI codes")
	}
}

func TestRenderFormSection_ContentPadding(t *testing.T) {
	// Content shorter than inner width should be padded
	content := []string{"Short"}
	result := RenderFormSection(content, "Title", "", 30, false, BorderHighlightFocusColor)

	// The result should maintain proper alignment
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 lines, got %d", len(lines))
	}

	// Check that content line has proper borders on both sides
	contentLine := lines[1]
	if !strings.Contains(contentLine, "│") {
		t.Error("Content line missing border characters")
	}

	// Should have border on left and right
	if contentLine[0] != '\x1b' && !strings.HasPrefix(contentLine, "│") {
		// Account for ANSI codes - the visual should still show borders
		if !strings.Contains(contentLine, "│") {
			t.Error("Content line should have vertical borders")
		}
	}
}

func TestRenderFormSection_HintFormatting(t *testing.T) {
	result := RenderFormSection([]string{"Content"}, "Title", "hint text", 40, false, BorderHighlightFocusColor)

	// Hint should be wrapped in parentheses
	if !strings.Contains(result, "(hint text)") {
		t.Error("Hint should be formatted with parentheses")
	}
}

func TestRenderFormSection_EmptyContent(t *testing.T) {
	// Empty content slice should still render borders
	result := RenderFormSection([]string{}, "Title", "", 30, false, BorderHighlightFocusColor)

	// Should have top and bottom borders
	if !strings.Contains(result, "╭") || !strings.Contains(result, "╰") {
		t.Error("Empty content should still have top and bottom borders")
	}
}

func TestRenderFormSection_LongTitle(t *testing.T) {
	// Title longer than available space
	longTitle := "This is a very long title that exceeds the available width"
	result := RenderFormSection([]string{"Content"}, longTitle, "", 30, false, BorderHighlightFocusColor)

	// Should still produce valid output with borders
	if !strings.Contains(result, "╭") || !strings.Contains(result, "╮") {
		t.Error("Long title should still produce valid borders")
	}

	// Should contain at least part of the title
	if !strings.Contains(result, "This") {
		t.Error("Should contain at least the beginning of the title")
	}
}
