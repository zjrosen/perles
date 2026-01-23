package formmodal

import (
	"fmt"
	"strings"

	zone "github.com/lrstanley/bubblezone"
	"github.com/muesli/reflow/wordwrap"

	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// Zone ID prefixes for mouse click detection.
const (
	zoneFieldPrefix      = "formmodal-field-"
	zoneFieldInputSuffix = "-input" // Suffix for input sub-section of editable list
	zoneItemPrefix       = "formmodal-item-"
	zoneSubmitButton     = "formmodal-submit"
	zoneCancelButton     = "formmodal-cancel"
)

// makeFieldZoneID creates a zone ID for a field.
func makeFieldZoneID(fieldIndex int) string {
	return fmt.Sprintf("%s%d", zoneFieldPrefix, fieldIndex)
}

// makeItemZoneID creates a zone ID for a list item within a field.
func makeItemZoneID(fieldIndex, itemIndex int) string {
	return fmt.Sprintf("%s%d-%d", zoneItemPrefix, fieldIndex, itemIndex)
}

// makeItemRowZoneID creates a zone ID for a specific row within a list item.
// Used for SearchSelect where items can have multiple rows (label + subtext lines).
// Format: formmodal-item-{fieldIndex}-{itemIndex}-r{rowIndex}
func makeItemRowZoneID(fieldIndex, itemIndex, rowIndex int) string {
	return fmt.Sprintf("%s%d-%d-r%d", zoneItemPrefix, fieldIndex, itemIndex, rowIndex)
}

// makeFieldInputZoneID creates a zone ID for the input sub-section of an editable list.
// Format: formmodal-field-{fieldIndex}-input
func makeFieldInputZoneID(fieldIndex int) string {
	return fmt.Sprintf("%s%d%s", zoneFieldPrefix, fieldIndex, zoneFieldInputSuffix)
}

// View renders the modal content (without overlay).
func (m *Model) View() string {
	width := m.config.MinWidth
	if width == 0 {
		width = 50
	}
	contentWidth := width - 2 // Account for modal border

	// Title with bottom border
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor)
	borderStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	titleBorder := borderStyle.Render(strings.Repeat("─", width))

	// Content style adds horizontal padding
	contentPadding := lipgloss.NewStyle().PaddingLeft(1)

	// Build title row (with optional right-aligned content)
	titleText := titleStyle.Render(m.config.Title)
	var titleRow string
	if m.config.TitleContent != nil {
		titleContent := m.config.TitleContent(contentWidth)
		if titleContent != "" {
			titleWidth := lipgloss.Width(titleText)
			contentWidth := lipgloss.Width(titleContent)
			availableWidth := width - 2 // Account for padding
			gap := max(availableWidth-titleWidth-contentWidth, 1)
			titleRow = titleText + strings.Repeat(" ", gap) + titleContent
		} else {
			titleRow = titleText
		}
	} else {
		titleRow = titleText
	}

	// Build content starting with title
	var content strings.Builder
	content.WriteString(contentPadding.Render(titleRow))
	content.WriteString("\n")
	content.WriteString(titleBorder)

	// Render optional header content
	if m.config.HeaderContent != nil {
		// Header content width accounts for content padding on each side
		headerWidth := max(contentWidth-2, 10)
		headerView := m.config.HeaderContent(headerWidth)
		if headerView != "" {
			// Single newline before header, then header adds spacing after
			content.WriteString("\n")
			headerStyle := lipgloss.NewStyle().Width(headerWidth)
			content.WriteString(contentPadding.Render(headerStyle.Render(headerView)))
			content.WriteString("\n\n")
		} else {
			content.WriteString("\n\n")
		}
	} else {
		content.WriteString("\n\n")
	}

	// Render scrollable body with fields
	bodyView := m.renderScrollableBody(contentWidth, contentPadding)
	content.WriteString(bodyView)

	// Validation error (if any)
	if m.validationError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(styles.StatusErrorColor).
			Width(contentWidth - 2) // Account for content padding
		content.WriteString(contentPadding.Render(" " + errorStyle.Render(m.validationError)))
		content.WriteString("\n\n")
	}

	// Buttons or loading indicator
	if m.loadingText != "" {
		loadingView := m.renderLoading(contentWidth)
		content.WriteString(contentPadding.Render(" " + loadingView))
	} else {
		buttonsView := m.renderButtons()
		content.WriteString(contentPadding.Render(" " + buttonsView))
	}
	content.WriteString("\n")

	// Wrap in bordered box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	return boxStyle.Render(content.String())
}

// renderScrollableBody renders the fields with scrolling applied via viewport.
func (m *Model) renderScrollableBody(contentWidth int, contentPadding lipgloss.Style) string {
	// Render all visible fields
	// Account for left padding (1 char) so fields don't overflow
	fieldWidth := contentWidth - 1
	var body strings.Builder
	for i := range m.fields {
		if !m.isFieldVisible(i) {
			continue
		}
		fieldView := m.renderField(i, fieldWidth)
		body.WriteString(contentPadding.Render(fieldView))
		body.WriteString("\n\n")
	}

	content := body.String()

	// If no size set, no scrolling - return content as-is
	if m.height == 0 {
		return content
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	contentHeight := len(lines)
	maxBodyHeight := m.calculateMaxBodyHeight()

	// If content fits, return as-is (modal will size to content)
	if contentHeight <= maxBodyHeight {
		return content
	}

	// Update viewport for scroll offset tracking
	m.bodyViewport.Height = maxBodyHeight
	m.bodyViewport.SetContent(content)

	// Extract visible lines based on viewport offset
	yOffset := m.bodyViewport.YOffset
	endLine := min(yOffset+maxBodyHeight, len(lines))
	visibleLines := lines[yOffset:endLine]

	// Pad to exactly maxBodyHeight lines to maintain consistent modal height
	for len(visibleLines) < maxBodyHeight {
		visibleLines = append(visibleLines, "")
	}

	return strings.Join(visibleLines, "\n") + "\n\n"
}

// renderField renders a single field based on its type.
// Fields are wrapped with zone marks for mouse click detection.
func (m Model) renderField(index int, width int) string {
	fs := &m.fields[index]
	cfg := fs.config
	focused := m.focusedIndex == index
	fieldZoneID := makeFieldZoneID(index)

	var rendered string
	switch cfg.Type {
	case FieldTypeText:
		// Set input width to fill available space
		// width - 2 for FormSection borders, - 1 for cursor padding
		fs.textInput.Width = width - 3
		rendered = styles.FormSection(styles.FormSectionConfig{
			Content:            []string{fs.textInput.View()},
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeColor:
		swatch := lipgloss.NewStyle().
			Background(lipgloss.Color(fs.selectedColor)).
			Render("  ")
		colorRow := swatch + " " + fs.selectedColor
		rendered = styles.FormSection(styles.FormSectionConfig{
			Content:            []string{colorRow},
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeList:
		var rows []string
		for i, item := range fs.listItems {
			prefix := " "
			if focused && i == fs.listCursor {
				prefix = styles.SelectionIndicatorStyle.Render(">")
			}
			checkbox := "[ ]"
			if item.selected {
				checkbox = "[x]"
			}
			row := prefix + checkbox + " " + item.label
			// Wrap the main row with zone mark for click detection
			rows = append(rows, zone.Mark(makeItemZoneID(index, i), row))
			// Add subtext on a separate line if present (not clickable separately)
			if item.subtext != "" {
				subtextStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)
				indent := "      " // Align with label after "[x] "
				rows = append(rows, indent+subtextStyle.Render(item.subtext))
			}
		}
		if len(rows) == 0 {
			rows = []string{" (no items)"}
		}
		rendered = styles.FormSection(styles.FormSectionConfig{
			Content:            rows,
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeSelect:
		var rows []string
		for i, item := range fs.listItems {
			prefix := " "
			if focused && i == fs.listCursor {
				prefix = styles.SelectionIndicatorStyle.Render(">")
			}
			radio := "( )"
			if item.selected {
				radio = "(●)"
			}
			// Apply color to label if specified
			label := item.label
			if item.color != nil {
				labelStyle := lipgloss.NewStyle().Foreground(item.color)
				if focused && i == fs.listCursor {
					labelStyle = labelStyle.Bold(true)
				}
				label = labelStyle.Render(item.label)
			}
			row := prefix + radio + " " + label
			// Wrap the main row with zone mark for click detection
			rows = append(rows, zone.Mark(makeItemZoneID(index, i), row))
			// Add subtext on a separate line if present (not clickable separately)
			if item.subtext != "" {
				subtextStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)
				indent := "      " // Align with label after "( ) "
				rows = append(rows, indent+subtextStyle.Render(item.subtext))
			}
		}
		if len(rows) == 0 {
			rows = []string{" (no items)"}
		}
		rendered = styles.FormSection(styles.FormSectionConfig{
			Content:            rows,
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeEditableList:
		rendered = m.renderEditableListField(fs, index, width, focused)
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeToggle:
		rendered = m.renderToggleField(fs, index, width, focused)
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeSearchSelect:
		rendered = m.renderSearchSelectField(fs, index, width, focused)
		return zone.Mark(fieldZoneID, rendered)

	case FieldTypeTextArea:
		rendered = m.renderTextAreaField(fs, width, focused)
		return zone.Mark(fieldZoneID, rendered)
	}

	return ""
}

// renderEditableListField renders the editable list as two adjacent sections.
// The list section shows items with checkboxes, the input section shows the add-item input.
func (m Model) renderEditableListField(fs *fieldState, fieldIndex int, width int, focused bool) string {
	cfg := fs.config

	// Determine which sub-section is focused
	listFocused := focused && fs.subFocus == SubFocusList
	inputFocused := focused && fs.subFocus == SubFocusInput

	// Render list section
	var listRows []string
	for i, item := range fs.listItems {
		prefix := " "
		if listFocused && i == fs.listCursor {
			prefix = styles.SelectionIndicatorStyle.Render(">")
		}
		checkbox := "[ ]"
		if item.selected {
			checkbox = "[x]"
		}
		row := prefix + checkbox + " " + item.label
		// Wrap with zone mark for click detection
		listRows = append(listRows, zone.Mark(makeItemZoneID(fieldIndex, i), row))
	}
	if len(listRows) == 0 {
		listRows = []string{" (no items)"}
	}
	listSection := styles.FormSection(styles.FormSectionConfig{
		Content:            listRows,
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		Focused:            listFocused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})

	// Render input section
	// Set input width: width - 2 (FormSection borders) - 1 (prefix) - 1 (cursor padding)
	fs.addInput.Width = width - 4
	inputPrefix := " "
	if inputFocused {
		inputPrefix = styles.SelectionIndicatorStyle.Render(">")
	}
	inputView := inputPrefix + fs.addInput.View()
	inputSection := styles.FormSection(styles.FormSectionConfig{
		Content:            []string{inputView},
		Width:              width,
		TopLeft:            cfg.InputLabel,
		TopLeftHint:        cfg.InputHint,
		Focused:            inputFocused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
	// Wrap input section with zone for click detection
	inputSection = zone.Mark(makeFieldInputZoneID(fieldIndex), inputSection)

	// Join with a small gap
	return lipgloss.JoinVertical(lipgloss.Left, listSection, "", inputSection)
}

// renderToggleField renders a binary toggle selector.
// Visual pattern matches coleditor: ● Selected    ○ Unselected [←/→]
func (m Model) renderToggleField(fs *fieldState, fieldIndex int, width int, focused bool) string {
	cfg := fs.config

	// Ensure we have exactly 2 options
	if len(cfg.Options) < 2 {
		return styles.FormSection(styles.FormSectionConfig{
			Content:            []string{"(invalid: need 2 options)"},
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})
	}

	selectedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextPrimaryColor)
	unselectedStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.TextMutedColor)

	var opt0Label, opt1Label string
	if fs.toggleIndex == 0 {
		opt0Label = selectedStyle.Render("● " + cfg.Options[0].Label)
		opt1Label = unselectedStyle.Render("○ " + cfg.Options[1].Label)
	} else {
		opt0Label = unselectedStyle.Render("○ " + cfg.Options[0].Label)
		opt1Label = selectedStyle.Render("● " + cfg.Options[1].Label)
	}

	// Wrap each option with zone mark for click detection
	opt0Label = zone.Mark(makeItemZoneID(fieldIndex, 0), opt0Label)
	opt1Label = zone.Mark(makeItemZoneID(fieldIndex, 1), opt1Label)

	hint := hintStyle.Render(" [←/→]")
	toggleRow := opt0Label + "    " + opt1Label + hint

	return styles.FormSection(styles.FormSectionConfig{
		Content:            []string{toggleRow},
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		Focused:            focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderButtons renders the submit and cancel buttons.
func (m Model) renderButtons() string {
	onButtons := m.focusedIndex == -1

	// Submit button
	submitLabel := m.config.SubmitLabel
	if submitLabel == "" {
		submitLabel = "Save"
	}
	var submitStyle lipgloss.Style
	switch m.config.SubmitVariant {
	case 1: // ButtonDanger - using literal to avoid import cycle
		submitStyle = styles.DangerButtonStyle
		if onButtons && m.focusedButton == 0 {
			submitStyle = styles.DangerButtonFocusedStyle
		}
	default: // ButtonPrimary
		submitStyle = styles.PrimaryButtonStyle
		if onButtons && m.focusedButton == 0 {
			submitStyle = styles.PrimaryButtonFocusedStyle
		}
	}
	submitBtn := zone.Mark(zoneSubmitButton, submitStyle.Render(submitLabel))

	// Cancel button
	cancelLabel := m.config.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}
	cancelStyle := styles.SecondaryButtonStyle
	if onButtons && m.focusedButton == 1 {
		cancelStyle = styles.SecondaryButtonFocusedStyle
	}
	cancelBtn := zone.Mark(zoneCancelButton, cancelStyle.Render(cancelLabel))

	return submitBtn + "  " + cancelBtn
}

// renderLoading renders the loading indicator centered in the available width.
func (m Model) renderLoading(contentWidth int) string {
	spinnerStyle := lipgloss.NewStyle().Foreground(styles.SpinnerColor)
	text := spinnerStyle.Render(m.loadingText)
	textWidth := lipgloss.Width(text)

	// Center the text within contentWidth
	leftPadding := max(0, (contentWidth-textWidth)/2)
	return strings.Repeat(" ", leftPadding) + text
}

// Overlay renders the modal on top of a background view.
func (m *Model) Overlay(bg string) string {
	fg := m.View()

	var result string
	if bg == "" {
		result = lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			fg,
		)
	} else {
		result = overlay.Place(overlay.Config{
			Width:    m.width,
			Height:   m.height,
			Position: overlay.Center,
		}, fg, bg)
	}

	// If colorpicker is open, layer it on top
	if m.showColorPicker {
		result = m.colorPicker.Overlay(result)
	}

	// Scan for zone markers to enable mouse click detection
	return zone.Scan(result)
}

// renderSearchSelectField renders the searchable select field.
// Has two states:
//   - Collapsed: Shows selected value with hint to press Enter to change
//   - Expanded: Shows search input + filtered list with scroll indicators
func (m Model) renderSearchSelectField(fs *fieldState, fieldIndex int, width int, focused bool) string {
	// Collapsed state: show selected value
	if !fs.searchExpanded {
		return m.renderSearchSelectCollapsed(fs, width, focused)
	}

	// Expanded state: show search + list
	return m.renderSearchSelectExpanded(fs, fieldIndex, width, focused)
}

// renderSearchSelectCollapsed renders the collapsed state showing selected value.
func (m Model) renderSearchSelectCollapsed(fs *fieldState, width int, focused bool) string {
	cfg := fs.config

	// Find selected item
	selectedLabel := "(none)"
	selectedSubtext := ""
	for _, item := range fs.listItems {
		if item.selected {
			selectedLabel = item.label
			selectedSubtext = item.subtext
			break
		}
	}

	// Calculate available width for label
	// width = contentWidth (modal width - 2 for modal border)
	// innerWidth = width - 2 for FormSection borders
	innerWidth := width - 2
	// Available for label: innerWidth - 1 (prefix space)
	availableWidth := innerWidth - 1

	// Truncate label if needed
	displayLabel := styles.TruncateString(selectedLabel, availableWidth)

	// Build content rows
	var rows []string
	rows = append(rows, " "+displayLabel)

	// Add wrapped subtext if present
	if selectedSubtext != "" {
		wrapWidth := innerWidth - 1
		if wrapWidth > 0 {
			wrapped := wordwrap.String(selectedSubtext, wrapWidth)
			subtextStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)
			for line := range strings.SplitSeq(wrapped, "\n") {
				rows = append(rows, subtextStyle.Render(" "+line))
			}
		}
	}

	return styles.FormSection(styles.FormSectionConfig{
		Content:            rows,
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		Focused:            focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderSearchSelectExpanded renders the expanded state with search input and list.
func (m Model) renderSearchSelectExpanded(fs *fieldState, fieldIndex int, width int, focused bool) string {
	cfg := fs.config
	maxVisible := cfg.MaxVisibleItems
	if maxVisible <= 0 {
		maxVisible = 5
	}

	// Divider spans full inner width (width - 2 for FormSection borders)
	innerWidth := width - 2

	// Search input row - set width to fill available space
	// innerWidth - 1 (prefix space) - 1 (cursor padding)
	fs.searchInput.Width = innerWidth - 2
	searchRow := " " + fs.searchInput.View()
	dividerStyle := lipgloss.NewStyle().Foreground(styles.BorderDefaultColor)
	divider := dividerStyle.Render(strings.Repeat("─", innerWidth))

	// Build content rows
	var rows []string
	rows = append(rows, searchRow)
	rows = append(rows, divider)

	if len(fs.searchFiltered) == 0 {
		noMatchStyle := lipgloss.NewStyle().
			Foreground(styles.TextMutedColor).
			Italic(true)
		rows = append(rows, noMatchStyle.Render(" No matches"))
	} else {
		endIdx := min(fs.scrollOffset+maxVisible, len(fs.searchFiltered))
		for i := fs.scrollOffset; i < endIdx; i++ {
			actualIdx := fs.searchFiltered[i]
			item := fs.listItems[actualIdx]

			// Highlight the cursor row (what user is about to select)
			isCursorRow := focused && i == fs.listCursor

			// Track row index within this item for unique zone IDs
			rowIdx := 0

			// Label row - padded to full width for larger click target
			displayLabel := styles.TruncateString(item.label, innerWidth-1)
			labelRow := " " + displayLabel
			// Always pad to full width so the entire row is clickable
			if lipgloss.Width(labelRow) < innerWidth {
				labelRow = labelRow + strings.Repeat(" ", innerWidth-lipgloss.Width(labelRow))
			}
			if isCursorRow {
				cursorStyle := lipgloss.NewStyle().Background(styles.SelectionBackgroundColor)
				labelRow = cursorStyle.Render(labelRow)
			}
			// Each row gets a unique zone ID: item-{field}-{item}-r{row}
			rows = append(rows, zone.Mark(makeItemRowZoneID(fieldIndex, actualIdx, rowIdx), labelRow))
			rowIdx++

			// Subtext rows (wrapped) - each gets its own unique zone ID
			if item.subtext != "" {
				wrapWidth := innerWidth - 4
				if wrapWidth > 0 {
					wrapped := wordwrap.String(item.subtext, wrapWidth)
					for line := range strings.SplitSeq(wrapped, "\n") {
						subtextRow := "   " + line
						// Pad to full width for clicking
						if lipgloss.Width(subtextRow) < innerWidth {
							subtextRow = subtextRow + strings.Repeat(" ", innerWidth-lipgloss.Width(subtextRow))
						}
						if isCursorRow {
							cursorStyle := lipgloss.NewStyle().
								Background(styles.SelectionBackgroundColor).
								Foreground(styles.TextDescriptionColor)
							subtextRow = cursorStyle.Render(subtextRow)
						} else {
							subtextStyle := lipgloss.NewStyle().Foreground(styles.TextDescriptionColor)
							subtextRow = subtextStyle.Render(subtextRow)
						}
						rows = append(rows, zone.Mark(makeItemRowZoneID(fieldIndex, actualIdx, rowIdx), subtextRow))
						rowIdx++
					}
				}
			}
		}

		// "More" indicator if there are items below
		if endIdx < len(fs.searchFiltered) {
			moreStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
			rows = append(rows, moreStyle.Render(" ↓ more..."))
		}
	}

	return styles.FormSection(styles.FormSectionConfig{
		Content:            rows,
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		Focused:            focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderTextAreaField renders the vimtextarea field.
func (m Model) renderTextAreaField(fs *fieldState, width int, focused bool) string {
	cfg := fs.config

	// Set textarea size: width - 2 for FormSection borders
	// Height defaults to MaxHeight from config, or 3 lines if not specified
	innerWidth := width - 2
	height := cfg.MaxHeight
	if height <= 0 {
		height = 3
	}
	fs.textArea.SetSize(innerWidth, height)

	// Get the textarea view and split into lines for FormSection
	view := fs.textArea.View()
	lines := strings.Split(view, "\n")

	// Ensure we have exactly 'height' lines for consistent box sizing
	for len(lines) < height {
		lines = append(lines, "")
	}

	// Get vim mode indicator for focused textarea
	var modeIndicator string
	if focused {
		modeIndicator = fs.textArea.ModeIndicator()
	}

	return styles.FormSection(styles.FormSectionConfig{
		Content:            lines,
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		BottomLeft:         modeIndicator,
		Focused:            focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}
