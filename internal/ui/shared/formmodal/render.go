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

	// Build content starting with title
	var content strings.Builder
	content.WriteString(contentPadding.Render(titleStyle.Render(m.config.Title)))
	content.WriteString("\n")
	content.WriteString(titleBorder)
	content.WriteString("\n\n")

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
			rows = append(rows, prefix+radio+" "+item.label)
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
