package formmodal

import (
	"strings"

	"github.com/zjrosen/perles/internal/ui/shared/overlay"
	"github.com/zjrosen/perles/internal/ui/styles"

	"github.com/charmbracelet/lipgloss"
)

// View renders the modal content (without overlay).
func (m Model) View() string {
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

	// Render each field
	for i := range m.fields {
		fieldView := m.renderField(i, contentWidth)
		content.WriteString(contentPadding.Render(fieldView))
		content.WriteString("\n\n")
	}

	// Validation error (if any)
	if m.validationError != "" {
		errorStyle := lipgloss.NewStyle().Foreground(styles.StatusErrorColor)
		content.WriteString(contentPadding.Render(" " + errorStyle.Render(m.validationError)))
		content.WriteString("\n\n")
	}

	// Buttons
	buttonsView := m.renderButtons()
	content.WriteString(contentPadding.Render(" " + buttonsView))
	content.WriteString("\n")

	// Wrap in bordered box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	return boxStyle.Render(content.String())
}

// renderField renders a single field based on its type.
func (m Model) renderField(index int, width int) string {
	fs := &m.fields[index]
	cfg := fs.config
	focused := m.focusedIndex == index

	switch cfg.Type {
	case FieldTypeText:
		return styles.FormSection(styles.FormSectionConfig{
			Content:            []string{fs.textInput.View()},
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})

	case FieldTypeColor:
		swatch := lipgloss.NewStyle().
			Background(lipgloss.Color(fs.selectedColor)).
			Render("  ")
		colorRow := swatch + " " + fs.selectedColor
		return styles.FormSection(styles.FormSectionConfig{
			Content:            []string{colorRow},
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})

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
			rows = append(rows, prefix+checkbox+" "+item.label)
		}
		if len(rows) == 0 {
			rows = []string{" (no items)"}
		}
		return styles.FormSection(styles.FormSectionConfig{
			Content:            rows,
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})

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
			rows = append(rows, prefix+radio+" "+label)
		}
		if len(rows) == 0 {
			rows = []string{" (no items)"}
		}
		return styles.FormSection(styles.FormSectionConfig{
			Content:            rows,
			Width:              width,
			TopLeft:            cfg.Label,
			TopLeftHint:        cfg.Hint,
			Focused:            focused,
			FocusedBorderColor: styles.BorderHighlightFocusColor,
		})

	case FieldTypeEditableList:
		return m.renderEditableListField(fs, width, focused)

	case FieldTypeToggle:
		return m.renderToggleField(fs, width, focused)

	case FieldTypeSearchSelect:
		return m.renderSearchSelectField(fs, width, focused)
	}

	return ""
}

// renderEditableListField renders the editable list as two adjacent sections.
// The list section shows items with checkboxes, the input section shows the add-item input.
func (m Model) renderEditableListField(fs *fieldState, width int, focused bool) string {
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
		listRows = append(listRows, prefix+checkbox+" "+item.label)
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

	// Join with a small gap
	return lipgloss.JoinVertical(lipgloss.Left, listSection, "", inputSection)
}

// renderToggleField renders a binary toggle selector.
// Visual pattern matches coleditor: ● Selected    ○ Unselected [←/→]
func (m Model) renderToggleField(fs *fieldState, width int, focused bool) string {
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
	submitBtn := submitStyle.Render(submitLabel)

	// Cancel button
	cancelLabel := m.config.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}
	cancelStyle := styles.SecondaryButtonStyle
	if onButtons && m.focusedButton == 1 {
		cancelStyle = styles.SecondaryButtonFocusedStyle
	}
	cancelBtn := cancelStyle.Render(cancelLabel)

	return submitBtn + "  " + cancelBtn
}

// Overlay renders the modal on top of a background view.
func (m Model) Overlay(bg string) string {
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

	return result
}

// renderSearchSelectField renders the searchable select field.
// Has two states:
//   - Collapsed: Shows selected value with hint to press Enter to change
//   - Expanded: Shows search input + filtered list with scroll indicators
func (m Model) renderSearchSelectField(fs *fieldState, width int, focused bool) string {
	// Collapsed state: show selected value
	if !fs.searchExpanded {
		return m.renderSearchSelectCollapsed(fs, width, focused)
	}

	// Expanded state: show search + list
	return m.renderSearchSelectExpanded(fs, width, focused)
}

// renderSearchSelectCollapsed renders the collapsed state showing selected value.
func (m Model) renderSearchSelectCollapsed(fs *fieldState, width int, focused bool) string {
	cfg := fs.config

	// Find selected item
	selectedLabel := "(none)"
	for _, item := range fs.listItems {
		if item.selected {
			selectedLabel = item.label
			break
		}
	}

	// Build content row (no indicator when collapsed, just indent)
	row := " " + selectedLabel

	// Add hint on the right
	hintStyle := lipgloss.NewStyle().Foreground(styles.TextMutedColor)
	hint := hintStyle.Render(" [enter to change]")
	row += hint

	return styles.FormSection(styles.FormSectionConfig{
		Content:            []string{row},
		Width:              width,
		TopLeft:            cfg.Label,
		TopLeftHint:        cfg.Hint,
		Focused:            focused,
		FocusedBorderColor: styles.BorderHighlightFocusColor,
	})
}

// renderSearchSelectExpanded renders the expanded state with search input and list.
func (m Model) renderSearchSelectExpanded(fs *fieldState, width int, focused bool) string {
	cfg := fs.config
	maxVisible := cfg.MaxVisibleItems
	if maxVisible <= 0 {
		maxVisible = 5
	}

	// Search input row
	searchRow := " " + fs.searchInput.View()

	// Divider spans full inner width (width - 2 for FormSection borders)
	innerWidth := width - 2
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

			// Build row content with padding
			rowContent := " " + item.label

			// Highlight the cursor row (what user is about to select)
			isCursorRow := focused && i == fs.listCursor
			if isCursorRow {
				// Pad to full width for consistent highlight
				rowWidth := lipgloss.Width(rowContent)
				if rowWidth < innerWidth {
					rowContent += strings.Repeat(" ", innerWidth-rowWidth)
				}
				cursorStyle := lipgloss.NewStyle().
					Background(styles.SelectionBackgroundColor).
					Foreground(lipgloss.Color("#FFFFFF"))
				rowContent = cursorStyle.Render(rowContent)
			}

			rows = append(rows, rowContent)
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
