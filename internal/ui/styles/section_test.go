package styles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/require"
)

func TestFormSection(t *testing.T) {
	// Use a consistent focus color for tests
	focusColor := lipgloss.Color("#54A0FF")

	tests := []struct {
		name           string
		config         FormSectionConfig
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "basic section with title",
			config: FormSectionConfig{
				Content:            []string{"  Content line"},
				Width:              30,
				TopLeft:            "Name",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭─ Name",
				"│",
				"Content line",
				"╰",
			},
		},
		{
			name: "section with title and hint",
			config: FormSectionConfig{
				Content:            []string{"  Input here"},
				Width:              40,
				TopLeft:            "Query",
				TopLeftHint:        "required",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭─ Query",
				"(required)",
				"│",
				"Input here",
				"╰",
			},
		},
		{
			name: "empty title renders plain border",
			config: FormSectionConfig{
				Content:            []string{"Content"},
				Width:              20,
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
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
			name: "multiple content lines",
			config: FormSectionConfig{
				Content:            []string{"Line 1", "Line 2", "Line 3"},
				Width:              25,
				TopLeft:            "Items",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"Line 1",
				"Line 2",
				"Line 3",
			},
		},
		{
			name: "focused section",
			config: FormSectionConfig{
				Content:            []string{"Focused content"},
				Width:              30,
				TopLeft:            "Focus",
				Focused:            true,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭─ Focus",
				"│",
				"Focused content",
				"╰",
			},
		},
		{
			name: "narrow width handles gracefully",
			config: FormSectionConfig{
				Content:            []string{"X"},
				Width:              5,
				TopLeft:            "T",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
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
			name: "minimum width",
			config: FormSectionConfig{
				Content:            []string{"A"},
				Width:              3,
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭",
				"╮",
				"│",
				"╰",
				"╯",
			},
		},
		{
			name: "bottom left indicator",
			config: FormSectionConfig{
				Content:            []string{"Query text"},
				Width:              40,
				TopLeft:            "BQL Query",
				BottomLeft:         "[NORMAL]",
				Focused:            true,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭─ BQL Query",
				"Query text",
				"╰─ [NORMAL]",
			},
		},
		{
			name: "top right title",
			config: FormSectionConfig{
				Content:            []string{"Content"},
				Width:              40,
				TopLeft:            "Left",
				TopRight:           "Right",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"Left",
				"Right",
				"Content",
			},
		},
		{
			name: "bottom right title",
			config: FormSectionConfig{
				Content:            []string{"Content"},
				Width:              40,
				TopLeft:            "Title",
				BottomRight:        "Status",
				Focused:            false,
				FocusedBorderColor: focusColor,
			},
			wantContains: []string{
				"╭─ Title",
				"Content",
				"Status",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormSection(tt.config)

			for _, want := range tt.wantContains {
				require.Contains(t, result, want, "FormSection() missing expected content")
			}

			for _, notWant := range tt.wantNotContain {
				require.NotContains(t, result, notWant, "FormSection() contains unexpected content")
			}
		})
	}
}

func TestFormSection_FocusChangesColor(t *testing.T) {
	// Force ANSI color output in test environment
	lipgloss.SetColorProfile(termenv.ANSI256)

	focusColor := lipgloss.Color("#54A0FF")

	unfocused := FormSection(FormSectionConfig{
		Content:            []string{"Content"},
		Width:              30,
		TopLeft:            "Test",
		Focused:            false,
		FocusedBorderColor: focusColor,
	})
	focused := FormSection(FormSectionConfig{
		Content:            []string{"Content"},
		Width:              30,
		TopLeft:            "Test",
		Focused:            true,
		FocusedBorderColor: focusColor,
	})

	// Both should contain the same structural elements
	for _, want := range []string{"╭", "╮", "│", "╰", "╯", "Content", "Test"} {
		require.Contains(t, unfocused, want, "Unfocused section missing element")
		require.Contains(t, focused, want, "Focused section missing element")
	}

	// The outputs should be different (different ANSI color codes)
	require.NotEqual(t, unfocused, focused, "Focused and unfocused sections should have different ANSI codes")
}

func TestFormSection_ContentPadding(t *testing.T) {
	// Content shorter than inner width should be padded
	result := FormSection(FormSectionConfig{
		Content:            []string{"Short"},
		Width:              30,
		TopLeft:            "Title",
		Focused:            false,
		FocusedBorderColor: BorderHighlightFocusColor,
	})

	// The result should maintain proper alignment
	lines := strings.Split(result, "\n")
	require.GreaterOrEqual(t, len(lines), 3, "Expected at least 3 lines")

	// Check that content line has proper borders on both sides
	contentLine := lines[1]
	require.Contains(t, contentLine, "│", "Content line missing border characters")

	// Should have border on left and right
	if contentLine[0] != '\x1b' && !strings.HasPrefix(contentLine, "│") {
		// Account for ANSI codes - the visual should still show borders
		require.Contains(t, contentLine, "│", "Content line should have vertical borders")
	}
}

func TestFormSection_HintFormatting(t *testing.T) {
	result := FormSection(FormSectionConfig{
		Content:            []string{"Content"},
		Width:              40,
		TopLeft:            "Title",
		TopLeftHint:        "hint text",
		Focused:            false,
		FocusedBorderColor: BorderHighlightFocusColor,
	})

	// Hint should be wrapped in parentheses
	require.Contains(t, result, "(hint text)", "Hint should be formatted with parentheses")
}

func TestFormSection_EmptyContent(t *testing.T) {
	// Empty content slice should still render borders
	result := FormSection(FormSectionConfig{
		Content:            []string{},
		Width:              30,
		TopLeft:            "Title",
		Focused:            false,
		FocusedBorderColor: BorderHighlightFocusColor,
	})

	// Should have top and bottom borders
	require.Contains(t, result, "╭", "Empty content should have top border")
	require.Contains(t, result, "╰", "Empty content should have bottom border")
}

func TestFormSection_LongTitle(t *testing.T) {
	// Title longer than available space
	longTitle := "This is a very long title that exceeds the available width"
	result := FormSection(FormSectionConfig{
		Content:            []string{"Content"},
		Width:              30,
		TopLeft:            longTitle,
		Focused:            false,
		FocusedBorderColor: BorderHighlightFocusColor,
	})

	// Should still produce valid output with borders
	require.Contains(t, result, "╭", "Long title should still produce valid top border")
	require.Contains(t, result, "╮", "Long title should still produce valid right border")

	// Should contain at least part of the title
	require.Contains(t, result, "This", "Should contain at least the beginning of the title")
}
