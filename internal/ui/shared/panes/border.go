// Package panes contains reusable bordered pane UI components.
package panes

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/ui/styles"
)

// Border characters (rounded)
const (
	borderTopLeft     = "╭"
	borderTopRight    = "╮"
	borderBottomLeft  = "╰"
	borderBottomRight = "╯"
	borderHorizontal  = "─"
	borderVertical    = "│"
	borderMiddleLeft  = "├"
	borderMiddleRight = "┤"
)

// Tab represents a single tab in tab mode.
// When BorderConfig.Tabs is non-empty, tab labels are rendered in the title bar
// and the active tab's Content is displayed inside the border.
type Tab struct {
	Label   string                 // Tab label displayed in title bar
	Content string                 // Pre-rendered content for this tab
	Color   lipgloss.TerminalColor // Optional custom color for this tab's label (nil = use default)
	ZoneID  string                 // Optional zone ID for mouse click detection (empty = no zone)
}

// BorderConfig configures the appearance of a bordered panel.
// This is the new struct-based API that replaces the 8-parameter RenderWithTitleBorder function.
type BorderConfig struct {
	// Required fields
	Content string // The content to render inside the border
	Width   int    // Total width including borders
	Height  int    // Total height including borders

	// Title placement (6 positions - all optional)
	TopLeft      string // Title on top border, left-aligned
	TopMiddle    string // Title on top border, centered (not yet implemented)
	TopRight     string // Title on top border, right-aligned
	BottomLeft   string // Title on bottom border, left-aligned
	BottomMiddle string // Title on bottom border, centered (not yet implemented)
	BottomRight  string // Title on bottom border, right-aligned

	// Styling
	Focused            bool                   // Whether the panel has focus
	TitleColor         lipgloss.TerminalColor // Color for title text
	BorderColor        lipgloss.TerminalColor // Border color when not focused
	FocusedBorderColor lipgloss.TerminalColor // Border color when focused

	// Tab mode (optional - when Tabs is non-empty, enables tab mode)
	// In tab mode, TopLeft, TopRight, and TitleColor are ignored.
	// Tab labels are rendered in the title bar instead.
	Tabs             []Tab                  // Enables tab mode when non-empty
	ActiveTab        int                    // 0-indexed active tab (clamped to valid range)
	ActiveTabColor   lipgloss.TerminalColor // Custom active tab color (default: BorderHighlightFocusColor)
	InactiveTabColor lipgloss.TerminalColor // Custom inactive tab color (default: TextMutedColor)

	// PreWrapped indicates the content already handles its own line wrapping.
	// When true, skips lipgloss Width constraint to avoid double-wrapping.
	// Use this for components like vimtextarea that manage their own soft-wrap.
	PreWrapped bool
}

// BorderedPane renders content within a bordered panel with optional titles.
// This is the new struct-based API. For backward compatibility, use RenderWithTitleBorder.
//
// Nil color fallback rules:
//   - Both BorderColor and FocusedBorderColor nil: use BorderDefaultColor for both states
//   - BorderColor set, FocusedBorderColor nil: inherit BorderColor for focused state
//   - BorderColor nil, FocusedBorderColor set: unfocused uses BorderDefaultColor, focused uses specified
//   - Both set: use appropriately based on Focused flag
//
// Tab mode:
//   - Enabled when len(cfg.Tabs) > 0
//   - In tab mode: TopLeft, TopRight, TitleColor are IGNORED
//   - Tab labels are rendered in the title bar with active/inactive styling
//   - Content is taken from cfg.Tabs[clampedActiveTab].Content
//   - BottomLeft and BottomRight continue to work normally in both modes
func BorderedPane(cfg BorderConfig) string {
	// Resolve border color with fallback logic
	borderColor := resolveBorderColor(cfg.BorderColor, cfg.FocusedBorderColor, cfg.Focused)

	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Calculate inner width (excluding border characters)
	innerWidth := max(cfg.Width-2, 1) // -2 for left and right border

	// Detect tab mode: len(cfg.Tabs) > 0
	isTabMode := len(cfg.Tabs) > 0

	var topBorder string
	var content string

	if isTabMode {
		// Tab mode: use tab styles and buildTabTitleTopBorder
		// TopLeft, TopRight, and TitleColor are IGNORED in tab mode

		// Clamp activeTab to valid range
		activeTab := max(cfg.ActiveTab, 0)
		if activeTab >= len(cfg.Tabs) {
			activeTab = len(cfg.Tabs) - 1
		}

		// Resolve tab styles
		// For the active tab, check if the tab has a custom color
		var activeTabColor lipgloss.TerminalColor
		if activeTab >= 0 && activeTab < len(cfg.Tabs) && cfg.Tabs[activeTab].Color != nil {
			activeTabColor = cfg.Tabs[activeTab].Color
		}
		activeStyle := resolveActiveTabStyle(cfg.ActiveTabColor, activeTabColor)
		inactiveStyle := resolveInactiveTabStyle(cfg.InactiveTabColor)

		// Build top border with tab labels
		topBorder = buildTabTitleTopBorder(cfg.Tabs, activeTab, innerWidth, borderStyle, activeStyle, inactiveStyle)

		// Select content from the active tab
		content = cfg.Tabs[activeTab].Content
	} else {
		// Classic mode: existing behavior unchanged
		// Resolve title color (default to BorderDefaultColor if nil)
		titleColor := cfg.TitleColor
		if titleColor == nil {
			titleColor = styles.BorderDefaultColor
		}
		titleStyle := lipgloss.NewStyle().Foreground(titleColor)

		// Build top border with embedded titles
		topBorder = buildDualTitleTopBorder(cfg.TopLeft, cfg.TopRight, innerWidth, borderStyle, titleStyle)

		// Use content from cfg.Content
		content = cfg.Content
	}

	// Build bottom border with embedded titles (handles empty titles correctly)
	// Format: ╰─ BottomLeft ─────────────────── BottomRight ─╯
	// Bottom titles work in both modes
	// Resolve title color for bottom border (default to BorderDefaultColor if nil)
	bottomTitleColor := cfg.TitleColor
	if bottomTitleColor == nil {
		bottomTitleColor = styles.BorderDefaultColor
	}
	bottomTitleStyle := lipgloss.NewStyle().Foreground(bottomTitleColor)
	bottomBorder := buildDualTitleBottomBorder(cfg.BottomLeft, cfg.BottomRight, innerWidth, borderStyle, bottomTitleStyle)

	// Calculate content height (excluding top and bottom borders)
	contentHeight := max(cfg.Height-2, 1)

	// Constrain content dimensions
	// When PreWrapped is true, skip lipgloss constraints entirely since content
	// handles its own wrapping (e.g., vimtextarea). The line loop below handles
	// height limiting and width padding. This avoids double-wrapping conflicts.
	var constrainedContent string
	if cfg.PreWrapped {
		constrainedContent = content
	} else {
		contentStyle := lipgloss.NewStyle().Width(innerWidth).Height(contentHeight)
		constrainedContent = contentStyle.Render(content)
	}

	// Split constrained content into lines
	contentLines := strings.Split(constrainedContent, "\n")
	paddedLines := make([]string, contentHeight)

	for i := range contentHeight {
		var line string
		if i < len(contentLines) {
			line = contentLines[i]
		}

		// Pad line to innerWidth to ensure right border aligns
		lineWidth := lipgloss.Width(line)
		if lineWidth < innerWidth {
			line = line + strings.Repeat(" ", innerWidth-lineWidth)
		}

		// Add side borders
		paddedLines[i] = borderStyle.Render(borderVertical) + line + borderStyle.Render(borderVertical)
	}

	// Join all parts
	var result strings.Builder
	result.WriteString(topBorder)
	result.WriteString("\n")
	result.WriteString(strings.Join(paddedLines, "\n"))
	result.WriteString("\n")
	result.WriteString(bottomBorder)

	return result.String()
}

// resolveBorderColor implements the nil color fallback logic for border colors.
//
// Fallback rules:
//   - Both nil: use BorderDefaultColor
//   - BorderColor set, FocusedBorderColor nil: inherit BorderColor for focused state
//   - BorderColor nil, FocusedBorderColor set: unfocused uses BorderDefaultColor, focused uses specified
//   - Both set: use appropriately based on focused flag
func resolveBorderColor(borderColor, focusedBorderColor lipgloss.TerminalColor, focused bool) lipgloss.TerminalColor {
	// Case 1: Both nil - use default
	if borderColor == nil && focusedBorderColor == nil {
		return styles.BorderDefaultColor
	}

	// Case 2: BorderColor set, FocusedBorderColor nil - inherit BorderColor
	if borderColor != nil && focusedBorderColor == nil {
		return borderColor
	}

	// Case 3: BorderColor nil, FocusedBorderColor set
	if borderColor == nil && focusedBorderColor != nil {
		if focused {
			return focusedBorderColor
		}
		return styles.BorderDefaultColor
	}

	// Case 4: Both set - use appropriately based on focused flag
	if focused {
		return focusedBorderColor
	}
	return borderColor
}

// resolveActiveTabStyle returns a bold style for active tab labels.
//
// Color precedence:
//   - tabColor (per-tab custom color) if set
//   - customColor (global active tab color) if set
//   - BorderHighlightFocusColor (default)
func resolveActiveTabStyle(customColor, tabColor lipgloss.TerminalColor) lipgloss.Style {
	// Determine color with precedence: tabColor > customColor > default
	var color lipgloss.TerminalColor = styles.BorderHighlightFocusColor
	if customColor != nil {
		color = customColor
	}
	if tabColor != nil {
		color = tabColor
	}
	return lipgloss.NewStyle().Bold(true).Foreground(color)
}

// resolveInactiveTabStyle returns a non-bold style for inactive tab labels.
//
// Color precedence:
//   - customColor (global inactive tab color) if set
//   - TextMutedColor (default)
func resolveInactiveTabStyle(customColor lipgloss.TerminalColor) lipgloss.Style {
	// Determine color with precedence: customColor > default
	var color lipgloss.TerminalColor = styles.TextMutedColor
	if customColor != nil {
		color = customColor
	}
	return lipgloss.NewStyle().Foreground(color)
}

// buildTopBorder creates the top border with embedded title.
// borderStyle is used for border characters, titleStyle for the title text.
func buildTopBorder(title string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	// Format: ╭─ Title ──────╮
	// Minimum: ╭─╮ (3 chars for just borders)

	if innerWidth < 1 {
		return borderStyle.Render(borderTopLeft + borderTopRight)
	}

	// If title is empty, just render a plain top border
	if title == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate space for title
	// Format: ─ Title ─
	// We need at least 4 chars: "─ " + " ─"
	titlePartMinWidth := 4

	if innerWidth < titlePartMinWidth {
		// Too narrow for title, just render plain border
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate available space for title text
	availableForTitle := innerWidth - 4 // "─ " before and " ─" after (minimum)

	displayTitle := title
	if lipgloss.Width(displayTitle) > availableForTitle {
		// Truncate title with ellipsis
		displayTitle = styles.TruncateString(displayTitle, availableForTitle)
	}

	// Calculate remaining width for trailing dashes
	// Inner: "─ " (2) + title + " " (1) + dashes = innerWidth
	// So: dashes = innerWidth - 3 - titleTextWidth
	titleTextWidth := lipgloss.Width(displayTitle)
	remainingWidth := max(innerWidth-3-titleTextWidth, 0)

	// Build: ╭─ Title ──────╮
	// Border parts use borderStyle, title text uses titleStyle
	return borderStyle.Render(borderTopLeft+borderHorizontal+" ") +
		titleStyle.Render(displayTitle) +
		borderStyle.Render(" "+strings.Repeat(borderHorizontal, remainingWidth)+borderTopRight)
}

// buildDualTitleTopBorder creates the top border with titles on both left and right.
// Format: ╭─ LeftTitle ─────────────────── RightTitle ─╮
func buildDualTitleTopBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if innerWidth < 1 {
		return borderStyle.Render(borderTopLeft + borderTopRight)
	}

	// If both titles are empty, just render a plain top border
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate widths
	leftTitleWidth := lipgloss.Width(leftTitle)
	rightTitleWidth := lipgloss.Width(rightTitle)

	// Minimum format: "─ Left ─ Right ─" = 2 + left + 1 + middle + 1 + right + 1
	// We need space for: "─ " + leftTitle + " " + middleDashes + " " + rightTitle + " ─"
	minRequired := 2 + leftTitleWidth + 1 + 1 + 1 + rightTitleWidth + 2
	if rightTitle == "" {
		// Just left title: "─ " + leftTitle + " " + dashes
		minRequired = 2 + leftTitleWidth + 1 + 1
	}
	if leftTitle == "" {
		// Just right title: dashes + " " + rightTitle + " ─"
		minRequired = 1 + 1 + rightTitleWidth + 2
	}

	if innerWidth < minRequired {
		// Too narrow, fall back to simple border or just left title
		if leftTitle != "" {
			return buildTopBorder(leftTitle, innerWidth, borderStyle, titleStyle)
		}
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate middle dashes
	// Format: ╭─ Left ───────── Right ─╮
	// innerWidth = 2 + leftWidth + 1 + middleDashes + 1 + rightWidth + 2
	// middleDashes = innerWidth - 2 - leftWidth - 1 - 1 - rightWidth - 2
	//              = innerWidth - leftWidth - rightWidth - 6
	var middleDashes int
	if leftTitle != "" && rightTitle != "" {
		middleDashes = innerWidth - leftTitleWidth - rightTitleWidth - 6
	} else if leftTitle != "" {
		// No right title: ╭─ Left ─────────╮
		middleDashes = innerWidth - leftTitleWidth - 3
	} else {
		// No left title: ╭───────── Right ─╮
		middleDashes = innerWidth - rightTitleWidth - 3
	}
	middleDashes = max(middleDashes, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderTopLeft))

	if leftTitle != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		result.WriteString(titleStyle.Render(leftTitle))
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, middleDashes)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(titleStyle.Render(rightTitle))
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderTopRight))

	return result.String()
}

// buildDualTitleBottomBorder creates the bottom border with titles on both left and right.
// Format: ╰─ LeftTitle ─────────────────── RightTitle ─╯
// This mirrors buildDualTitleTopBorder but uses bottom corner characters.
func buildDualTitleBottomBorder(leftTitle, rightTitle string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	if innerWidth < 1 {
		return borderStyle.Render(borderBottomLeft + borderBottomRight)
	}

	// If both titles are empty, just render a plain bottom border
	if leftTitle == "" && rightTitle == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate widths
	leftTitleWidth := lipgloss.Width(leftTitle)
	rightTitleWidth := lipgloss.Width(rightTitle)

	// Minimum format: "─ Left ─ Right ─" = 2 + left + 1 + middle + 1 + right + 1
	// We need space for: "─ " + leftTitle + " " + middleDashes + " " + rightTitle + " ─"
	minRequired := 2 + leftTitleWidth + 1 + 1 + 1 + rightTitleWidth + 2
	if rightTitle == "" {
		// Just left title: "─ " + leftTitle + " " + dashes
		minRequired = 2 + leftTitleWidth + 1 + 1
	}
	if leftTitle == "" {
		// Just right title: dashes + " " + rightTitle + " ─"
		minRequired = 1 + 1 + rightTitleWidth + 2
	}

	if innerWidth < minRequired {
		// Too narrow, fall back to simple border or just left title
		if leftTitle != "" {
			return buildBottomBorder(leftTitle, innerWidth, borderStyle, titleStyle)
		}
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate middle dashes
	// Format: ╰─ Left ───────── Right ─╯
	// innerWidth = 2 + leftWidth + 1 + middleDashes + 1 + rightWidth + 2
	// middleDashes = innerWidth - 2 - leftWidth - 1 - 1 - rightWidth - 2
	//              = innerWidth - leftWidth - rightWidth - 6
	var middleDashes int
	if leftTitle != "" && rightTitle != "" {
		middleDashes = innerWidth - leftTitleWidth - rightTitleWidth - 6
	} else if leftTitle != "" {
		// No right title: ╰─ Left ─────────╯
		middleDashes = innerWidth - leftTitleWidth - 3
	} else {
		// No left title: ╰───────── Right ─╯
		middleDashes = innerWidth - rightTitleWidth - 3
	}
	middleDashes = max(middleDashes, 1)

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderBottomLeft))

	if leftTitle != "" {
		result.WriteString(borderStyle.Render(borderHorizontal + " "))
		result.WriteString(titleStyle.Render(leftTitle))
		result.WriteString(borderStyle.Render(" "))
	}

	result.WriteString(borderStyle.Render(strings.Repeat(borderHorizontal, middleDashes)))

	if rightTitle != "" {
		result.WriteString(borderStyle.Render(" "))
		result.WriteString(titleStyle.Render(rightTitle))
		result.WriteString(borderStyle.Render(" " + borderHorizontal))
	}

	result.WriteString(borderStyle.Render(borderBottomRight))

	return result.String()
}

// buildTabTitleTopBorder creates the top border with tab labels.
// Format: ╭─ Tab1 ─ Tab2 ─ Tab3 ───────────────╮
// The active tab receives activeStyle (bold), inactive tabs receive inactiveStyle.
// Labels are truncated if they exceed available width.
func buildTabTitleTopBorder(tabs []Tab, activeTab int, innerWidth int, borderStyle, activeStyle, inactiveStyle lipgloss.Style) string {
	if innerWidth < 1 {
		return borderStyle.Render(borderTopLeft + borderTopRight)
	}

	// If no tabs, render plain border
	if len(tabs) == 0 {
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Clamp activeTab to valid range: [0, len(tabs)-1]
	if activeTab < 0 {
		activeTab = 0
	}
	if activeTab >= len(tabs) {
		activeTab = len(tabs) - 1
	}

	// Separator between tabs: " ─ " (3 chars visual width)
	separator := " " + borderHorizontal + " "
	separatorWidth := 3

	// Calculate available width for all tab labels
	// Format: ╭─ Tab1 ─ Tab2 ─ Tab3 ───╮
	// Fixed parts: "─ " prefix (2 chars) + " ─" suffix (2 chars) = 4 chars minimum
	// Separators: (numTabs - 1) * 3 chars for " ─ " between tabs
	numTabs := len(tabs)
	fixedOverhead := 2 // "─ " prefix before first tab
	if numTabs > 1 {
		fixedOverhead += (numTabs - 1) * separatorWidth
	}
	// We need at least 1 dash at the end before the corner
	minTrailingDashes := 1

	availableForLabels := innerWidth - fixedOverhead - minTrailingDashes
	if availableForLabels < numTabs {
		// Not enough space for even 1 char per tab - fall back to plain border
		return borderStyle.Render(borderTopLeft + strings.Repeat(borderHorizontal, innerWidth) + borderTopRight)
	}

	// Calculate total label width
	totalLabelWidth := 0
	for _, tab := range tabs {
		totalLabelWidth += lipgloss.Width(tab.Label)
	}

	// If labels exceed available space, truncate them proportionally
	labels := make([]string, numTabs)
	if totalLabelWidth > availableForLabels {
		// Truncate labels proportionally
		// Simple approach: give each label a proportional share based on its original length
		for i, tab := range tabs {
			originalWidth := lipgloss.Width(tab.Label)
			// Calculate this label's share of available space (min 3 for "..." truncation, max originalWidth)
			share := min(max(availableForLabels*originalWidth/totalLabelWidth, 3), originalWidth)
			labels[i] = styles.TruncateString(tab.Label, share)
		}
	} else {
		// No truncation needed
		for i, tab := range tabs {
			labels[i] = tab.Label
		}
	}

	// Build the border
	var result strings.Builder
	result.WriteString(borderStyle.Render(borderTopLeft + borderHorizontal + " "))

	// Render each tab label with appropriate style
	// If label contains ANSI escape codes (pre-styled), don't apply additional styling
	// This allows callers to pre-style labels with colored indicators + muted text
	for i, label := range labels {
		var styledLabel string
		if strings.Contains(label, "\x1b[") {
			// Pre-styled label - use as-is
			styledLabel = label
		} else {
			// Plain text - apply active/inactive styling
			var style lipgloss.Style
			if i == activeTab {
				style = activeStyle
			} else {
				style = inactiveStyle
			}
			styledLabel = style.Render(label)
		}

		// Wrap with zone mark if ZoneID is set
		if i < len(tabs) && tabs[i].ZoneID != "" {
			styledLabel = zone.Mark(tabs[i].ZoneID, styledLabel)
		}

		result.WriteString(styledLabel)

		// Add separator after all but the last tab
		if i < numTabs-1 {
			result.WriteString(borderStyle.Render(separator))
		}
	}

	// Calculate trailing dashes
	// Total rendered so far: "╭─ " (3) + labels + separators
	renderedContentWidth := 2 // "─ " prefix
	for _, label := range labels {
		renderedContentWidth += lipgloss.Width(label)
	}
	if numTabs > 1 {
		renderedContentWidth += (numTabs - 1) * separatorWidth
	}

	trailingDashes := max(innerWidth-renderedContentWidth, 1)

	// Add space before trailing dashes, then dashes, then corner
	result.WriteString(borderStyle.Render(" " + strings.Repeat(borderHorizontal, trailingDashes-1) + borderTopRight))

	return result.String()
}

// buildBottomBorder creates the bottom border with embedded title.
// borderStyle is used for border characters, titleStyle for the title text.
// This mirrors buildTopBorder but uses bottom corner characters.
func buildBottomBorder(title string, innerWidth int, borderStyle, titleStyle lipgloss.Style) string {
	// Format: ╰─ Title ──────╯
	// Minimum: ╰─╯ (3 chars for just borders)

	if innerWidth < 1 {
		return borderStyle.Render(borderBottomLeft + borderBottomRight)
	}

	// If title is empty, just render a plain bottom border
	if title == "" {
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate space for title
	// Format: ─ Title ─
	// We need at least 4 chars: "─ " + " ─"
	titlePartMinWidth := 4

	if innerWidth < titlePartMinWidth {
		// Too narrow for title, just render plain border
		return borderStyle.Render(borderBottomLeft + strings.Repeat(borderHorizontal, innerWidth) + borderBottomRight)
	}

	// Calculate available space for title text
	availableForTitle := innerWidth - 4 // "─ " before and " ─" after (minimum)

	displayTitle := title
	if lipgloss.Width(displayTitle) > availableForTitle {
		// Truncate title with ellipsis
		displayTitle = styles.TruncateString(displayTitle, availableForTitle)
	}

	// Calculate remaining width for trailing dashes
	// Inner: "─ " (2) + title + " " (1) + dashes = innerWidth
	// So: dashes = innerWidth - 3 - titleTextWidth
	titleTextWidth := lipgloss.Width(displayTitle)
	remainingWidth := max(innerWidth-3-titleTextWidth, 0)

	// Build: ╰─ Title ──────╯
	// Border parts use borderStyle, title text uses titleStyle
	return borderStyle.Render(borderBottomLeft+borderHorizontal+" ") +
		titleStyle.Render(displayTitle) +
		borderStyle.Render(" "+strings.Repeat(borderHorizontal, remainingWidth)+borderBottomRight)
}
