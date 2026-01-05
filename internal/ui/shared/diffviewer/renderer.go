package diffviewer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/ui/styles"
)

// lineNumberWidth is the width reserved for line numbers in the gutter.
const lineNumberWidth = 4

// Tree rendering constants
const (
	treeExpandedIcon  = "▼"
	treeCollapsedIcon = "▶"
	treeIndentWidth   = 2 // spaces per depth level
)

// renderFileTreeNode renders a single node in the file tree.
// Shows expand/collapse indicator for directories, status indicator for files.
// The focused parameter controls whether to show selection highlight.
func renderFileTreeNode(node *FileTreeNode, selected, focused bool, width int) string {
	if width < 1 {
		return ""
	}

	// Calculate indent based on depth
	indent := strings.Repeat(" ", node.Depth*treeIndentWidth)
	indentWidth := lipgloss.Width(indent)

	// Build the line components
	var prefix string
	var prefixStyle lipgloss.Style

	if node.IsDir {
		// Directory: show expand/collapse indicator
		if node.Expanded {
			prefix = treeExpandedIcon
		} else {
			prefix = treeCollapsedIcon
		}
		prefixStyle = lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	} else {
		// File: show status indicator
		indicator := getNodeIndicator(node)
		prefix = indicator
		prefixStyle = getNodeIndicatorStyle(node)
	}

	prefixWidth := lipgloss.Width(prefix)

	// Get stats for files only (not directories to avoid repetitive noise)
	var statsStr string
	if !node.IsDir && node.File != nil {
		statsStr = formatStats(node.File.Additions, node.File.Deletions, node.File.IsBinary)
	}
	statsWidth := lipgloss.Width(statsStr)

	// Calculate available width for name
	// Format: "indent prefix name stats"
	// indent + prefix + space + name + space + stats
	nameMaxWidth := width - indentWidth - prefixWidth - 1 - statsWidth
	if statsWidth > 0 {
		nameMaxWidth-- // space before stats
	}
	nameMaxWidth = max(nameMaxWidth, 1)

	// Truncate name if needed
	name := node.Name
	if lipgloss.Width(name) > nameMaxWidth {
		name = ansi.Truncate(name, nameMaxWidth, "")
	}

	// Calculate padding
	nameWidth := lipgloss.Width(name)
	padding := max(nameMaxWidth-nameWidth, 0)

	// Build the styled line
	bgColor := styles.SelectionBackgroundColor

	if selected && focused {
		// Apply background to all parts
		indentStyle := lipgloss.NewStyle().Background(bgColor)
		prefixStyleSelected := prefixStyle.Background(bgColor)
		nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)

		var result strings.Builder
		result.WriteString(indentStyle.Render(indent))
		result.WriteString(prefixStyleSelected.Render(prefix))
		result.WriteString(spaceStyle.Render(" "))
		result.WriteString(nameStyle.Render(name))
		result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))
		if statsStr != "" {
			result.WriteString(spaceStyle.Render(" "))
			// Stats need special handling - they have their own colors
			// For selected state, we render stats with background
			result.WriteString(formatStatsWithBackground(node, bgColor))
		}
		return result.String()
	}

	// Non-selected rendering
	nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	if node.IsDir {
		nameStyle = nameStyle.Bold(true)
	}

	var result strings.Builder
	result.WriteString(indent)
	result.WriteString(prefixStyle.Render(prefix))
	result.WriteString(" ")
	result.WriteString(nameStyle.Render(name))
	result.WriteString(strings.Repeat(" ", padding))
	if statsStr != "" {
		result.WriteString(" ")
		result.WriteString(statsStr)
	}
	return result.String()
}

// getNodeIndicatorStyle returns the style for a file tree node indicator.
func getNodeIndicatorStyle(node *FileTreeNode) lipgloss.Style {
	if node.IsDir || node.File == nil {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	}
	return getStatusIndicatorStyle(getFileStatus(node.File))
}

// formatStatsWithBackground formats stats with a background color for selected state.
func formatStatsWithBackground(node *FileTreeNode, bgColor lipgloss.AdaptiveColor) string {
	var adds, dels int
	var binary bool

	if node.IsDir {
		adds, dels = node.TotalStats()
	} else if node.File != nil {
		adds = node.File.Additions
		dels = node.File.Deletions
		binary = node.File.IsBinary
	}

	if binary {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Background(bgColor).Render("binary")
	}

	addStyle := lipgloss.NewStyle().Foreground(styles.DiffAdditionColor).Background(bgColor)
	delStyle := lipgloss.NewStyle().Foreground(styles.DiffDeletionColor).Background(bgColor)
	spaceStyle := lipgloss.NewStyle().Background(bgColor)

	var parts []string
	if adds > 0 {
		parts = append(parts, addStyle.Render(fmt.Sprintf("+%d", adds)))
	}
	if dels > 0 {
		parts = append(parts, delStyle.Render(fmt.Sprintf("-%d", dels)))
	}

	// Join with background-styled spaces
	if len(parts) == 2 {
		return parts[0] + spaceStyle.Render(" ") + parts[1]
	} else if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

// renderFileTree renders the complete file tree content.
// Returns the inner content to be wrapped in a BorderedPane.
func renderFileTree(tree *FileTree, selectedIndex, scrollTop, width, height int, focused bool) string {
	if height < 1 || width < 1 {
		return ""
	}

	nodes := tree.VisibleNodes()

	if len(nodes) == 0 {
		return renderNoFilesPlaceholder(width, height)
	}

	var lines []string
	visibleEnd := min(scrollTop+height, len(nodes))

	for i := scrollTop; i < visibleEnd; i++ {
		node := nodes[i]
		selected := i == selectedIndex
		line := renderFileTreeNode(node, selected, focused, width)
		lines = append(lines, line)
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// formatStats formats the +N/-N display for a file.
func formatStats(additions, deletions int, binary bool) string {
	if binary {
		return lipgloss.NewStyle().Foreground(styles.TextMutedColor).Render("binary")
	}

	addStyle := lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
	delStyle := lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)

	var parts []string
	if additions > 0 {
		parts = append(parts, addStyle.Render(fmt.Sprintf("+%d", additions)))
	}
	if deletions > 0 {
		parts = append(parts, delStyle.Render(fmt.Sprintf("-%d", deletions)))
	}

	return strings.Join(parts, " ")
}

// formatGutter formats the line number gutter.
// Shows line number for unified diff view (prefers new line number, falls back to old).
func formatGutter(oldLineNum, newLineNum int) string {
	// For unified diff view, show the relevant line number
	// Additions use new line number, deletions use old, context shows new
	if newLineNum > 0 {
		return fmt.Sprintf("%4d | ", newLineNum)
	} else if oldLineNum > 0 {
		return fmt.Sprintf("%4d | ", oldLineNum)
	}
	return "     | "
}

// renderBinaryPlaceholder renders a placeholder for binary files.
func renderBinaryPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "Binary file - cannot display diff", styles.TextMutedColor)
}

// renderNoChangesPlaceholder renders a placeholder for files with no diff content.
func renderNoChangesPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "No changes to display", styles.TextMutedColor)
}

// renderCommitListItem renders a single commit entry for the commit picker pane.
// Shows short hash (colored by sync status) and subject (truncated).
// Hash color: green for pushed, yellow for unpushed.
// Selection highlights the full line width with a background color (only when focused).
// Format: "abc1234 Fix auth bug"
func renderCommitListItem(commit git.CommitInfo, selected, focused bool, width int) string {
	if width < 1 {
		return ""
	}

	// Short hash - green for pushed, yellow for unpushed
	var hashColor lipgloss.AdaptiveColor
	if commit.IsPushed {
		hashColor = styles.StatusSuccessColor
	} else {
		hashColor = styles.StatusWarningColor
	}
	hashWidth := lipgloss.Width(commit.ShortHash)

	// Calculate available width for subject
	// Format: "hash subject"
	// hash + space + subject
	fixedWidth := hashWidth + 1 // hash + space
	subjectMaxWidth := max(width-fixedWidth, 1)

	// Truncate subject if needed (no ellipsis, just cut off)
	subject := commit.Subject
	if lipgloss.Width(subject) > subjectMaxWidth {
		subject = ansi.Truncate(subject, subjectMaxWidth, "")
	}

	// Calculate padding to fill the full width
	contentWidth := hashWidth + 1 + lipgloss.Width(subject)
	padding := max(width-contentWidth, 0)

	// Build the line content (without styling first)
	var content strings.Builder
	content.WriteString(commit.ShortHash)
	content.WriteString(" ")
	content.WriteString(subject)
	content.WriteString(strings.Repeat(" ", padding))

	if selected && focused {
		// For selected lines, apply background to each part directly
		// (wrapping already-styled text with background doesn't propagate through ANSI sequences)
		bgColor := styles.SelectionBackgroundColor
		hashStyle := lipgloss.NewStyle().Foreground(hashColor).Background(bgColor).Bold(true)
		subjectStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)

		var result strings.Builder
		result.WriteString(hashStyle.Render(commit.ShortHash))
		result.WriteString(spaceStyle.Render(" "))
		result.WriteString(subjectStyle.Render(subject))
		result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))

		return result.String()
	}

	// Non-selected: just style hash and subject
	hashStyle := lipgloss.NewStyle().Foreground(hashColor)
	subjectStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)

	var result strings.Builder
	result.WriteString(hashStyle.Render(commit.ShortHash))
	result.WriteString(" ")
	result.WriteString(subjectStyle.Render(subject))
	result.WriteString(strings.Repeat(" ", padding))

	return result.String()
}

// renderCommitList renders the commit list content (without borders).
// Returns the inner content to be wrapped in a BorderedPane.
// The focused parameter controls whether selection highlight is shown.
func renderCommitList(commits []git.CommitInfo, selectedCommit, scrollTop, width, height int, focused bool) string {
	if height < 1 || width < 1 {
		return ""
	}

	// Handle empty commits (empty repo)
	if len(commits) == 0 {
		return renderNoCommitsPlaceholder(width, height)
	}

	var lines []string

	// Calculate visible window
	visibleEnd := min(scrollTop+height, len(commits))

	for i := scrollTop; i < visibleEnd; i++ {
		commit := commits[i]
		selected := i == selectedCommit
		line := renderCommitListItem(commit, selected, focused, width)
		lines = append(lines, line)
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// renderNoCommitsPlaceholder renders a placeholder for empty repository.
func renderNoCommitsPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "No commits yet", styles.TextMutedColor)
}

// renderEnhancedEmptyState renders an informative empty state with helpful tips.
// This is shown when there are no changes to display (working directory is clean).
func renderEnhancedEmptyState(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	titleStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	tipStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor)

	keyStyle := lipgloss.NewStyle().
		Foreground(styles.DiffHunkColor)

	// Build content with tips
	var lines []string
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("No changes to display"))
	lines = append(lines, "")
	lines = append(lines, subtitleStyle.Render("Your working directory is clean."))
	lines = append(lines, "")
	lines = append(lines, tipStyle.Render("Tips:"))
	lines = append(lines, tipStyle.Render("• Make changes to files and return here"))
	lines = append(lines, tipStyle.Render("• Use "+keyStyle.Render("[")+" / "+keyStyle.Render("]")+" to switch between Commits, Branches, and Worktrees tabs"))
	lines = append(lines, tipStyle.Render("• Press "+keyStyle.Render("[?]")+" for help"))
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Center the content block
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}

// renderErrorState renders an error state with recovery actions.
// Shows the error message and available recovery options based on error category.
func renderErrorState(diffErr DiffError, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	warningStyle := lipgloss.NewStyle().
		Foreground(styles.StatusErrorColor).
		Bold(true)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextPrimaryColor)

	helpStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	keyStyle := lipgloss.NewStyle().
		Foreground(styles.DiffHunkColor)

	// Build error content
	var lines []string
	lines = append(lines, "")
	lines = append(lines, warningStyle.Render("⚠ "+diffErr.Category.String()))
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render(diffErr.Message))

	// Add help text if available
	if diffErr.HelpText != "" {
		lines = append(lines, helpStyle.Render(diffErr.HelpText))
	}

	lines = append(lines, "")

	// Add recovery actions based on category
	recoveryActions := getRecoveryActions(diffErr.Category)
	for _, action := range recoveryActions {
		lines = append(lines, keyStyle.Render(action.Key)+" "+helpStyle.Render(action.Description))
	}

	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Center the content block
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}

// recoveryAction represents a keyboard action for error recovery.
type recoveryAction struct {
	Key         string
	Description string
}

// getRecoveryActions returns the appropriate recovery actions for an error category.
func getRecoveryActions(category ErrorCategory) []recoveryAction {
	// Common help action
	helpAction := recoveryAction{Key: "[?]", Description: "Get help"}

	switch category {
	case ErrCategoryParse:
		return []recoveryAction{
			{Key: "[v]", Description: "View raw output"},
			helpAction,
		}
	case ErrCategoryGitOp:
		return []recoveryAction{
			{Key: "[r]", Description: "Retry"},
			helpAction,
		}
	case ErrCategoryPermission:
		return []recoveryAction{
			helpAction,
		}
	case ErrCategoryConflict:
		return []recoveryAction{
			{Key: "[r]", Description: "Reload"},
			helpAction,
		}
	case ErrCategoryTimeout:
		return []recoveryAction{
			{Key: "[r]", Description: "Retry with longer timeout"},
			helpAction,
		}
	default:
		return []recoveryAction{
			{Key: "[r]", Description: "Retry"},
			helpAction,
		}
	}
}

// renderEnhancedLoadingState renders a loading state with spinner and context.
// Shows what operation is being performed and provides a better user experience.
func renderEnhancedLoadingState(width, height int, message string) string {
	if width < 1 || height < 1 {
		return ""
	}

	if message == "" {
		message = "Loading diff..."
	}

	spinnerStyle := lipgloss.NewStyle().
		Foreground(styles.SpinnerColor)

	messageStyle := lipgloss.NewStyle().
		Foreground(styles.TextSecondaryColor)

	// Build loading content
	var lines []string
	lines = append(lines, "")
	lines = append(lines, spinnerStyle.Render("⏳ "+message))
	lines = append(lines, "")
	lines = append(lines, messageStyle.Render("This may take a moment for large diffs"))
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Center the content block
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}

// renderDiffContentWithWordDiff renders diff content with word-level highlighting.
// If wordDiff is nil or empty, falls back to standard line-level rendering.
// Pass height=0 to render all content (viewport handles scrolling).
func renderDiffContentWithWordDiff(file DiffFile, wordDiff *fileWordDiff, width, height int) string {
	if width < 1 {
		return ""
	}

	// Handle binary files
	if file.IsBinary {
		return renderBinaryPlaceholder(width, height)
	}

	// Handle files with no hunks (metadata-only changes like permission changes)
	if len(file.Hunks) == 0 {
		return renderNoChangesPlaceholder(width, height)
	}

	// Define styles using diff tokens
	addStyle := lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
	delStyle := lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)
	contextStyle := lipgloss.NewStyle().Foreground(styles.DiffContextColor)
	hunkStyle := lipgloss.NewStyle().Foreground(styles.DiffHunkColor)
	gutterStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Word-level highlight styles (background highlights for changed portions)
	wordAddStyle := lipgloss.NewStyle().
		Foreground(styles.DiffAdditionColor).
		Background(styles.DiffWordAdditionBgColor)
	wordDelStyle := lipgloss.NewStyle().
		Foreground(styles.DiffDeletionColor).
		Background(styles.DiffWordDeletionBgColor)

	// Calculate content width (subtract gutter)
	// Format: "NNNN | content" (4 digits + space + pipe + space + content)
	gutterWidth := lineNumberWidth + 3 // "NNNN | "
	contentWidth := max(width-gutterWidth, 1)

	var lines []string

	for hunkIdx, hunk := range file.Hunks {
		for lineIdx, line := range hunk.Lines {
			var renderedLine string

			switch line.Type {
			case LineHunkHeader:
				// Hunk headers don't have line numbers
				headerText := hunk.Header
				if len(headerText) > width {
					headerText = ansi.Truncate(headerText, width, "...")
				}
				renderedLine = hunkStyle.Render(headerText)

			case LineAddition:
				gutter := formatGutter(0, line.NewLineNum)
				prefix := "+"

				// Check if word diff is available for this line
				var segments []wordSegment
				if wordDiff != nil {
					segments = wordDiff.getSegmentsForLine(hunkIdx, lineIdx, LineAddition)
				}

				var styledContent string
				if len(segments) > 0 {
					// Render with word-level highlighting
					styledContent = renderSegments(segments, addStyle, wordAddStyle)
				} else {
					// Fall back to standard line coloring
					styledContent = addStyle.Render(line.Content)
				}

				fullLine := addStyle.Render(prefix) + styledContent
				if lipgloss.Width(fullLine) > contentWidth {
					fullLine = ansi.Truncate(fullLine, contentWidth, "")
				}
				renderedLine = gutterStyle.Render(gutter) + fullLine

			case LineDeletion:
				gutter := formatGutter(line.OldLineNum, 0)
				prefix := "-"

				// Check if word diff is available for this line
				var segments []wordSegment
				if wordDiff != nil {
					segments = wordDiff.getSegmentsForLine(hunkIdx, lineIdx, LineDeletion)
				}

				var styledContent string
				if len(segments) > 0 {
					// Render with word-level highlighting
					styledContent = renderSegments(segments, delStyle, wordDelStyle)
				} else {
					// Fall back to standard line coloring
					styledContent = delStyle.Render(line.Content)
				}

				fullLine := delStyle.Render(prefix) + styledContent
				if lipgloss.Width(fullLine) > contentWidth {
					fullLine = ansi.Truncate(fullLine, contentWidth, "")
				}
				renderedLine = gutterStyle.Render(gutter) + fullLine

			case LineContext:
				gutter := formatGutter(line.OldLineNum, line.NewLineNum)
				prefix := " "
				content := line.Content
				fullLine := prefix + content
				if len(fullLine) > contentWidth {
					fullLine = ansi.Truncate(fullLine, contentWidth, "")
				}
				renderedLine = gutterStyle.Render(gutter) + contextStyle.Render(fullLine)
			}

			lines = append(lines, renderedLine)
		}
	}

	return strings.Join(lines, "\n")
}

// renderSegments renders a slice of word segments with appropriate styling.
// unchangedStyle is used for unchanged segments, changedStyle for changed ones.
func renderSegments(segments []wordSegment, unchangedStyle, changedStyle lipgloss.Style) string {
	var result strings.Builder

	for _, seg := range segments {
		switch seg.Type {
		case segmentUnchanged:
			result.WriteString(unchangedStyle.Render(seg.Text))
		case segmentAdded, segmentDeleted:
			result.WriteString(changedStyle.Render(seg.Text))
		}
	}

	return result.String()
}

// Side-by-side rendering constants
const (
	sideBySideSeparator     = "│"
	sideBySideMinColWidth   = 40 // Minimum width for each column
	sideBySideGutterWidth   = 5  // "NNNN " for line numbers
	sideBySideHeaderPadding = 1  // Padding around OLD/NEW headers
)

// renderDiffContentSideBySide renders diff content in side-by-side two-column layout.
// Left column shows old file (deletions), right column shows new file (additions).
// Uses alignHunk to pair lines from old and new versions.
func renderDiffContentSideBySide(file DiffFile, wordDiff *fileWordDiff, width, height int) string {
	if width < 1 {
		return ""
	}

	// Handle binary files
	if file.IsBinary {
		return renderBinaryPlaceholder(width, height)
	}

	// Handle files with no hunks (metadata-only changes like permission changes)
	if len(file.Hunks) == 0 {
		return renderNoChangesPlaceholder(width, height)
	}

	// Check minimum width for side-by-side view
	// Need room for: gutter + content + separator + gutter + content
	minWidth := sideBySideGutterWidth + sideBySideMinColWidth + 1 + sideBySideGutterWidth + sideBySideMinColWidth
	if width < minWidth {
		// Fall back to unified view for narrow terminals
		return renderDiffContentWithWordDiff(file, wordDiff, width, height)
	}

	// Calculate column widths
	// Layout: [gutter][content] │ [gutter][content]
	// Each side gets half the width minus the separator
	sideWidth := (width - 1) / 2 // -1 for separator
	leftGutter := sideBySideGutterWidth
	rightGutter := sideBySideGutterWidth
	leftContentWidth := max(sideWidth-leftGutter, 1)
	rightContentWidth := max(sideWidth-rightGutter, 1)

	// Define styles
	addStyle := lipgloss.NewStyle().Foreground(styles.DiffAdditionColor)
	delStyle := lipgloss.NewStyle().Foreground(styles.DiffDeletionColor)
	contextStyle := lipgloss.NewStyle().Foreground(styles.DiffContextColor)
	hunkStyle := lipgloss.NewStyle().Foreground(styles.DiffHunkColor)
	gutterStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	separatorStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	// Word-level highlight styles
	wordAddStyle := lipgloss.NewStyle().
		Foreground(styles.DiffAdditionColor).
		Background(styles.DiffWordAdditionBgColor)
	wordDelStyle := lipgloss.NewStyle().
		Foreground(styles.DiffDeletionColor).
		Background(styles.DiffWordDeletionBgColor)

	var lines []string

	// Process each hunk
	for hunkIdx, hunk := range file.Hunks {
		// Align the hunk lines into pairs
		pairs := alignHunk(hunk)

		for _, pair := range pairs {
			var leftSide, rightSide string

			switch {
			case pair.IsHunkHeader():
				// Render hunk header spanning left side, empty on right
				headerText := hunk.Header
				if len(headerText) > sideWidth {
					headerText = ansi.Truncate(headerText, sideWidth, "...")
				}
				leftSide = hunkStyle.Render(padRight(headerText, sideWidth))
				rightSide = strings.Repeat(" ", sideWidth)

			case pair.IsContext():
				// Both sides show the same content
				leftSide = renderSideBySideLine(
					pair.Left, leftContentWidth,
					contextStyle, gutterStyle, nil, contextStyle,
				)
				rightSide = renderSideBySideLine(
					pair.Right, rightContentWidth,
					contextStyle, gutterStyle, nil, contextStyle,
				)

			case pair.IsModification():
				// Left shows deletion, right shows addition with word diff
				var leftSegments, rightSegments []wordSegment
				if wordDiff != nil {
					// Find the line indices for word diff lookup
					leftIdx := findLineIndex(hunk.Lines, pair.Left)
					rightIdx := findLineIndex(hunk.Lines, pair.Right)
					if leftIdx >= 0 {
						leftSegments = wordDiff.getSegmentsForLine(hunkIdx, leftIdx, LineDeletion)
					}
					if rightIdx >= 0 {
						rightSegments = wordDiff.getSegmentsForLine(hunkIdx, rightIdx, LineAddition)
					}
				}
				leftSide = renderSideBySideLine(
					pair.Left, leftContentWidth,
					delStyle, gutterStyle, leftSegments, wordDelStyle,
				)
				rightSide = renderSideBySideLine(
					pair.Right, rightContentWidth,
					addStyle, gutterStyle, rightSegments, wordAddStyle,
				)

			case pair.IsDeletion():
				// Left shows deletion, right is blank
				var segments []wordSegment
				if wordDiff != nil {
					lineIdx := findLineIndex(hunk.Lines, pair.Left)
					if lineIdx >= 0 {
						segments = wordDiff.getSegmentsForLine(hunkIdx, lineIdx, LineDeletion)
					}
				}
				leftSide = renderSideBySideLine(
					pair.Left, leftContentWidth,
					delStyle, gutterStyle, segments, wordDelStyle,
				)
				rightSide = renderSideBySideEmptyLine(rightGutter, rightContentWidth, emptyStyle, gutterStyle)

			case pair.IsAddition():
				// Left is blank, right shows addition
				var segments []wordSegment
				if wordDiff != nil {
					lineIdx := findLineIndex(hunk.Lines, pair.Right)
					if lineIdx >= 0 {
						segments = wordDiff.getSegmentsForLine(hunkIdx, lineIdx, LineAddition)
					}
				}
				leftSide = renderSideBySideEmptyLine(leftGutter, leftContentWidth, emptyStyle, gutterStyle)
				rightSide = renderSideBySideLine(
					pair.Right, rightContentWidth,
					addStyle, gutterStyle, segments, wordAddStyle,
				)
			}

			lines = append(lines, leftSide+separatorStyle.Render(sideBySideSeparator)+rightSide)
		}
	}

	return strings.Join(lines, "\n")
}

// renderSideBySideLine renders a single line for side-by-side view.
// Returns a string of exactly (sideBySideGutterWidth + contentWidth) width.
func renderSideBySideLine(
	line *DiffLine,
	contentWidth int,
	lineStyle, gutterStyle lipgloss.Style,
	segments []wordSegment,
	wordStyle lipgloss.Style,
) string {
	if line == nil {
		return strings.Repeat(" ", sideBySideGutterWidth+contentWidth)
	}

	// Format gutter (line number)
	var lineNum int
	if line.Type == LineDeletion {
		lineNum = line.OldLineNum
	} else {
		lineNum = line.NewLineNum
	}

	gutter := fmt.Sprintf("%4d ", lineNum)
	if lineNum == 0 {
		gutter = "     "
	}

	// Render content with optional word diff
	var contentStr string
	if len(segments) > 0 {
		contentStr = renderSegments(segments, lineStyle, wordStyle)
	} else {
		contentStr = lineStyle.Render(line.Content)
	}

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

	return gutterStyle.Render(gutter) + contentStr
}

// renderSideBySideEmptyLine renders an empty line (blank placeholder) for side-by-side view.
// Used when one side has content and the other is empty.
func renderSideBySideEmptyLine(gutterWidth, contentWidth int, emptyStyle, gutterStyle lipgloss.Style) string {
	gutter := strings.Repeat(" ", gutterWidth)
	content := strings.Repeat(" ", contentWidth)
	return gutterStyle.Render(gutter) + emptyStyle.Render(content)
}

// padRight pads a string with spaces to reach the target width.
func padRight(s string, width int) string {
	currentWidth := lipgloss.Width(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}

// findLineIndex finds the index of a line in the hunk's Lines slice.
func findLineIndex(lines []DiffLine, target *DiffLine) int {
	if target == nil {
		return -1
	}
	for i := range lines {
		if &lines[i] == target {
			return i
		}
	}
	return -1
}

// renderBranchListItem renders a single branch entry for the branches tab.
// Shows selection indicator (>) and (current) marker for the checked-out branch.
// Selection highlights the full line width with a background color (only when focused).
// Format: "> branch-name (current)" or "  branch-name"
func renderBranchListItem(branch git.BranchInfo, selected, focused bool, width int) string {
	if width < 1 {
		return ""
	}

	// Current branch marker
	currentMarker := ""
	if branch.IsCurrent {
		currentMarker = " (current)"
	}
	markerWidth := lipgloss.Width(currentMarker)

	// Calculate available width for branch name
	// Format: "name marker"
	nameMaxWidth := max(width-markerWidth, 1)

	// Truncate name if needed
	name := branch.Name
	if lipgloss.Width(name) > nameMaxWidth {
		name = ansi.Truncate(name, nameMaxWidth, "")
	}

	// Calculate padding to fill the full width
	contentWidth := lipgloss.Width(name) + markerWidth
	padding := max(width-contentWidth, 0)

	if selected && focused {
		bgColor := styles.SelectionBackgroundColor
		nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)
		markerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Background(bgColor)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)

		var result strings.Builder
		result.WriteString(nameStyle.Render(name))
		result.WriteString(markerStyle.Render(currentMarker))
		result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))

		return result.String()
	}

	// Non-selected rendering
	nameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	markerStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	var result strings.Builder
	result.WriteString(nameStyle.Render(name))
	result.WriteString(markerStyle.Render(currentMarker))
	result.WriteString(strings.Repeat(" ", padding))

	return result.String()
}

// renderBranchList renders the branch list content (without borders).
// Returns the inner content to be wrapped in a BorderedPane.
// The focused parameter controls whether selection highlight is shown.
func renderBranchList(branches []git.BranchInfo, selectedBranch, scrollTop, width, height int, focused bool) string {
	if height < 1 || width < 1 {
		return ""
	}

	// Handle empty branches
	if len(branches) == 0 {
		return renderNoBranchesPlaceholder(width, height)
	}

	var lines []string

	// Calculate visible window
	visibleEnd := min(scrollTop+height, len(branches))

	for i := scrollTop; i < visibleEnd; i++ {
		branch := branches[i]
		selected := i == selectedBranch
		line := renderBranchListItem(branch, selected, focused, width)
		lines = append(lines, line)
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// renderNoBranchesPlaceholder renders a placeholder when there are no branches.
func renderNoBranchesPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "No branches", styles.TextMutedColor)
}

// renderWorktreeListItem renders a single worktree entry for the worktrees tab.
// Shows selection indicator (>) and displays as "dirname (branch)" format.
// Selection highlights the full line width with a background color (only when focused).
// Format: "> worktree-name (branch-name)" or "  worktree-name (branch-name)"
func renderWorktreeListItem(worktree git.WorktreeInfo, selected, focused bool, width int) string {
	if width < 1 {
		return ""
	}

	// Get directory basename from path
	dirname := worktree.Path
	if lastSlash := strings.LastIndex(worktree.Path, "/"); lastSlash >= 0 {
		dirname = worktree.Path[lastSlash+1:]
	}

	// Branch info in parentheses
	branchInfo := ""
	if worktree.Branch != "" {
		branchInfo = " (" + worktree.Branch + ")"
	}
	branchInfoWidth := lipgloss.Width(branchInfo)

	// Calculate available width for dirname
	// Format: "dirname branchInfo"
	dirnameMaxWidth := max(width-branchInfoWidth, 1)

	// Truncate dirname if needed
	if lipgloss.Width(dirname) > dirnameMaxWidth {
		dirname = ansi.Truncate(dirname, dirnameMaxWidth, "")
	}

	// Calculate padding to fill the full width
	contentWidth := lipgloss.Width(dirname) + branchInfoWidth
	padding := max(width-contentWidth, 0)

	if selected && focused {
		bgColor := styles.SelectionBackgroundColor
		dirnameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor).Background(bgColor).Bold(true)
		branchStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor).Background(bgColor)
		spaceStyle := lipgloss.NewStyle().Background(bgColor)

		var result strings.Builder
		result.WriteString(dirnameStyle.Render(dirname))
		result.WriteString(branchStyle.Render(branchInfo))
		result.WriteString(spaceStyle.Render(strings.Repeat(" ", padding)))

		return result.String()
	}

	// Non-selected rendering
	dirnameStyle := lipgloss.NewStyle().Foreground(styles.TextPrimaryColor)
	branchStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)

	var result strings.Builder
	result.WriteString(dirnameStyle.Render(dirname))
	result.WriteString(branchStyle.Render(branchInfo))
	result.WriteString(strings.Repeat(" ", padding))

	return result.String()
}

// renderWorktreeList renders the worktree list content (without borders).
// Returns the inner content to be wrapped in a BorderedPane.
// The focused parameter controls whether selection highlight is shown.
func renderWorktreeList(worktrees []git.WorktreeInfo, selectedWorktree, scrollTop, width, height int, focused bool) string {
	if height < 1 || width < 1 {
		return ""
	}

	// Handle empty worktrees
	if len(worktrees) == 0 {
		return renderNoWorktreesPlaceholder(width, height)
	}

	var lines []string

	// Calculate visible window
	visibleEnd := min(scrollTop+height, len(worktrees))

	for i := scrollTop; i < visibleEnd; i++ {
		worktree := worktrees[i]
		selected := i == selectedWorktree
		line := renderWorktreeListItem(worktree, selected, focused, width)
		lines = append(lines, line)
	}

	// Pad to full height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

// renderNoWorktreesPlaceholder renders a placeholder when there are no worktrees.
func renderNoWorktreesPlaceholder(width, height int) string {
	return renderCenteredPlaceholder(width, height, "No worktrees", styles.TextMutedColor)
}
