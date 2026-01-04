// Package panes contains reusable bordered pane UI components.
package panes

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// Scroll indicator styles
var (
	// ScrollIndicatorStyle is the style for scroll position indicators (e.g., "↑50%").
	// Uses muted text color for subtlety.
	ScrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(styles.TextMutedColor)

	// NewContentIndicatorStyle is the style for the "↓New" indicator shown when
	// new content arrives while scrolled up. Uses attention-grabbing yellow/amber.
	NewContentIndicatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#FECA57", Dark: "#FECA57"}).
					Bold(true)
)

// ScrollableConfig holds the configuration for rendering a scrollable pane.
// This is the shared version extracted from orchestration mode for reuse
// across different parts of the codebase.
type ScrollableConfig struct {
	// Viewport is a pointer to the viewport model.
	// CRITICAL: Must be a pointer to preserve reference semantics for scroll state persistence.
	// The viewport will be modified by ScrollablePane (dimensions, content, scroll position).
	Viewport *viewport.Model

	// ContentDirty indicates whether the content has changed since last render.
	// Used to determine if auto-scroll to bottom should occur.
	ContentDirty bool

	// HasNewContent indicates if new content arrived while scrolled up.
	// Displayed as "↓New" indicator in the right title.
	HasNewContent bool

	// MetricsDisplay is optional metrics text (e.g., "27k/200k" for context).
	// Displayed in the right title.
	MetricsDisplay string

	// LeftTitle is the title shown on the left side of the border.
	LeftTitle string

	// BottomLeft is optional text shown on the bottom-left of the border.
	// Useful for status indicators like queue counts.
	BottomLeft string

	// TitleColor is the color for the title text.
	TitleColor lipgloss.AdaptiveColor

	// BorderColor is the color for the pane border.
	BorderColor lipgloss.AdaptiveColor

	// Focused indicates whether the pane has focus.
	// Passed through to BorderedPane for border styling.
	Focused bool
}

// ScrollablePane handles the common viewport setup, content padding, auto-scroll,
// and border rendering pattern used by scrollable pane components.
//
// This function composes with BorderedPane internally to render the final output.
//
// CRITICAL INVARIANTS (do not change the order of operations):
//  1. wasAtBottom MUST be captured BEFORE SetContent() to preserve user scroll position.
//     If checked after SetContent(), users will be forcibly scrolled to bottom on every render.
//  2. Content padding MUST be PREPENDED (not appended) to push content to the bottom of the viewport.
//     Appending padding would leave content at the top.
//  3. Viewport MUST use pointer semantics (stored in map) for scroll state to persist across renders.
//
// contentFn receives the available width (viewport width) and returns the rendered content string.
func ScrollablePane(
	width, height int,
	cfg ScrollableConfig,
	contentFn func(wrapWidth int) string,
) string {
	// Calculate viewport dimensions (subtract 2 for borders)
	vpWidth := max(width-2, 1)
	vpHeight := max(height-2, 1)

	// Build pre-wrapped content
	content := contentFn(vpWidth)

	// Pad content to push it to the bottom when it's shorter than viewport.
	// CRITICAL: Padding must be PREPENDED to push content to bottom.
	// This preserves the "latest content at bottom" behavior.
	contentLines := strings.Split(content, "\n")
	if len(contentLines) < vpHeight {
		padding := make([]string, vpHeight-len(contentLines))
		contentLines = append(padding, contentLines...) // Prepend padding
		content = strings.Join(contentLines, "\n")
	}

	// Capture scroll state BEFORE dimension/content changes for proportional preservation.
	// This must happen before any viewport mutations.
	wasAtBottom := cfg.Viewport.AtBottom()
	oldScrollPercent := cfg.Viewport.ScrollPercent()
	dimensionsChanged := cfg.Viewport.Width != vpWidth || cfg.Viewport.Height != vpHeight

	// Update viewport dimensions
	cfg.Viewport.Width = vpWidth
	cfg.Viewport.Height = vpHeight

	cfg.Viewport.SetContent(content)

	// Scroll position restoration (order matters):
	// 1. If at bottom, stay at bottom (live view experience)
	// 2. If dimensions changed, restore proportional scroll position
	// 3. Otherwise, scroll position is preserved by SetContent's internal offset handling
	if wasAtBottom {
		cfg.Viewport.GotoBottom()
	} else if dimensionsChanged && oldScrollPercent > 0 {
		// Restore proportional scroll position after resize
		totalLines := cfg.Viewport.TotalLineCount()
		scrollableRange := totalLines - cfg.Viewport.Height
		if scrollableRange > 0 {
			newOffset := int(oldScrollPercent * float64(scrollableRange))
			cfg.Viewport.SetYOffset(newOffset)
		}
	}

	// Get viewport view (handles scrolling and clipping)
	viewportContent := cfg.Viewport.View()

	// Build right title with new content indicator, scroll indicator, and metrics
	// This must happen AFTER SetContent so scroll indicator is accurate
	rightTitle := buildRightTitle(*cfg.Viewport, cfg.HasNewContent, cfg.MetricsDisplay)

	// Render pane with bordered title using the BorderedPane API
	return BorderedPane(BorderConfig{
		Content:     viewportContent,
		Width:       width,
		Height:      height,
		TopLeft:     cfg.LeftTitle,
		TopRight:    rightTitle,
		BottomLeft:  cfg.BottomLeft,
		Focused:     cfg.Focused,
		TitleColor:  cfg.TitleColor,
		BorderColor: cfg.BorderColor,
	})
}

// buildRightTitle constructs the right title section for pane borders.
// It combines the new content indicator, scroll indicator, and optional metrics display.
func buildRightTitle(vp viewport.Model, hasNewContent bool, metricsDisplay string) string {
	var parts []string

	// Add new content indicator if scrolled up and new content arrived
	if hasNewContent {
		parts = append(parts, NewContentIndicatorStyle.Render("↓New"))
	}

	// Add scroll indicator if scrolled up from bottom
	if scrollIndicator := BuildScrollIndicator(vp); scrollIndicator != "" {
		parts = append(parts, scrollIndicator)
	}

	// Add metrics display if available (e.g., "27k/200k" for context usage)
	if metricsDisplay != "" {
		parts = append(parts, ScrollIndicatorStyle.Render(metricsDisplay))
	}

	return strings.Join(parts, " ")
}

// BuildScrollIndicator returns a styled scroll position indicator for the viewport.
// Returns empty string if content fits in viewport or if at bottom (live view).
// Returns styled "↑XX%" when scrolled up from bottom.
//
// This function is exported for use by external packages that may need to build
// custom scroll indicators or test the scroll indicator logic.
func BuildScrollIndicator(vp viewport.Model) string {
	if vp.TotalLineCount() <= vp.Height {
		return "" // Content fits, no indicator needed
	}
	if vp.AtBottom() {
		return "" // At live position, no indicator needed
	}
	return ScrollIndicatorStyle.Render(fmt.Sprintf("↑%.0f%%", vp.ScrollPercent()*100))
}
