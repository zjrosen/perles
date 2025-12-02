// Package colorpicker provides a visual color selection component.
package colorpicker

import (
	"perles/internal/ui/overlay"
	"perles/internal/ui/styles"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PresetColor represents a named color option.
type PresetColor struct {
	Name string
	Hex  string // e.g., "#FF8787"
}

// DefaultPresets is a curated palette of colors (Column 1).
var DefaultPresets = []PresetColor{
	{Name: "Red", Hex: "#FF8787"},
	{Name: "Green", Hex: "#73F59F"},
	{Name: "Blue", Hex: "#54A0FF"},
	{Name: "Purple", Hex: "#7D56F4"},
	{Name: "Yellow", Hex: "#FECA57"},
	{Name: "Orange", Hex: "#FF9F43"},
	{Name: "Teal", Hex: "#89DCEB"},
	{Name: "Gray", Hex: "#BBBBBB"},
	{Name: "Pink", Hex: "#CBA6F7"},
	{Name: "Coral", Hex: "#FF6B6B"},
}

// Column2Presets provides additional color options (Column 2).
var Column2Presets = []PresetColor{
	{Name: "Lime", Hex: "#A3E635"},
	{Name: "Cyan", Hex: "#22D3EE"},
	{Name: "Magenta", Hex: "#E879F9"},
	{Name: "Indigo", Hex: "#818CF8"},
	{Name: "Rose", Hex: "#FB7185"},
	{Name: "Amber", Hex: "#FBBF24"},
	{Name: "Emerald", Hex: "#34D399"},
	{Name: "Sky", Hex: "#38BDF8"},
	{Name: "Fuchsia", Hex: "#D946EF"},
	{Name: "Violet", Hex: "#A78BFA"},
}

// Column3Presets provides more color options (Column 3).
var Column3Presets = []PresetColor{
	{Name: "Crimson", Hex: "#DC143C"},
	{Name: "Tomato", Hex: "#FF6347"},
	{Name: "Gold", Hex: "#FFD700"},
	{Name: "Olive", Hex: "#808000"},
	{Name: "Aqua", Hex: "#00FFFF"},
	{Name: "Navy", Hex: "#000080"},
	{Name: "Plum", Hex: "#DDA0DD"},
	{Name: "Salmon", Hex: "#FA8072"},
	{Name: "Khaki", Hex: "#F0E68C"},
	{Name: "Mint", Hex: "#98FF98"},
}

// GrayscalePresets provides grayscale options (Column 4).
var GrayscalePresets = []PresetColor{
	{Name: "White", Hex: "#FFFFFF"},
	{Name: "Gray 1", Hex: "#E5E5E5"},
	{Name: "Gray 2", Hex: "#CCCCCC"},
	{Name: "Gray 3", Hex: "#B3B3B3"},
	{Name: "Gray 4", Hex: "#999999"},
	{Name: "Gray 5", Hex: "#808080"},
	{Name: "Gray 6", Hex: "#666666"},
	{Name: "Gray 7", Hex: "#4D4D4D"},
	{Name: "Gray 8", Hex: "#333333"},
	{Name: "Black", Hex: "#000000"},
}

// Custom mode focus fields.
const (
	customFocusInput = iota
	customFocusSave
	customFocusCancel
)

// Model holds the color picker state.
type Model struct {
	columns         [][]PresetColor // All preset columns
	column          int             // Current column (0-2)
	selected        int             // Selected row within current column
	customEnabled   bool
	customInput     textinput.Model
	inCustomMode    bool
	customFocus     int  // Which element is focused in custom mode
	showCustomError bool // Show error after Save clicked with invalid hex
	viewportWidth   int
	viewportHeight  int
	boxWidth        int
}

// SelectMsg is sent when a color is selected.
type SelectMsg struct {
	Hex string
}

// CancelMsg is sent when the picker is cancelled.
type CancelMsg struct{}

// New creates a new color picker with default presets.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "#RRGGBB"
	ti.CharLimit = 7
	ti.Width = 10
	ti.Prompt = ""

	return Model{
		columns: [][]PresetColor{
			DefaultPresets,
			Column2Presets,
			Column3Presets,
			GrayscalePresets,
		},
		column:        0,
		selected:      0,
		customEnabled: true,
		customInput:   ti,
		inCustomMode:  false,
		boxWidth:      64, // 4 columns × 16 = 64 chars
	}
}

// SetSize sets the viewport dimensions for overlay rendering.
func (m Model) SetSize(width, height int) Model {
	m.viewportWidth = width
	m.viewportHeight = height
	return m
}

// SetSelected finds and selects the color matching the given hex value.
// Also resets to preset selection mode (exits custom mode if active).
// If the color is not found in presets, defaults to the first selection.
func (m Model) SetSelected(hex string) Model {
	// Always reset to preset selection mode when setting a color
	m.inCustomMode = false
	m.customFocus = customFocusInput
	m.showCustomError = false
	m.customInput.Blur()

	for col, presets := range m.columns {
		for row, preset := range presets {
			if strings.EqualFold(preset.Hex, hex) {
				m.column = col
				m.selected = row
				return m
			}
		}
	}
	// Not found - custom color, default to first selection
	m.column = 0
	m.selected = 0
	return m
}

// SetBoxWidth sets the width of the picker box.
func (m Model) SetBoxWidth(width int) Model {
	m.boxWidth = width
	return m
}

// Selected returns the currently selected preset.
func (m Model) Selected() PresetColor {
	if m.column >= 0 && m.column < len(m.columns) {
		presets := m.columns[m.column]
		if m.selected >= 0 && m.selected < len(presets) {
			return presets[m.selected]
		}
	}
	return PresetColor{}
}

// InCustomMode returns whether the picker is in custom hex entry mode.
func (m Model) InCustomMode() bool {
	return m.inCustomMode
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.inCustomMode {
		return m.updateCustomMode(msg)
	}
	return m.updateNormalMode(msg)
}

func (m Model) updateNormalMode(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		currentColumn := m.columns[m.column]
		switch msg.String() {
		case "j", "down", "ctrl+n":
			if m.selected < len(currentColumn)-1 {
				m.selected++
			}
		case "k", "up", "ctrl+p":
			if m.selected > 0 {
				m.selected--
			}
		case "h", "left":
			if m.column > 0 {
				m.column--
				// Clamp selected to new column's bounds
				newColumn := m.columns[m.column]
				if m.selected >= len(newColumn) {
					m.selected = len(newColumn) - 1
				}
			}
		case "l", "right":
			if m.column < len(m.columns)-1 {
				m.column++
				// Clamp selected to new column's bounds
				newColumn := m.columns[m.column]
				if m.selected >= len(newColumn) {
					m.selected = len(newColumn) - 1
				}
			}
		case "enter":
			return m, selectCmd(currentColumn[m.selected].Hex)
		case "esc":
			return m, cancelCmd()
		case "c":
			if m.customEnabled {
				m.inCustomMode = true
				m.customInput.SetValue("")
				m.customInput.Focus()
				return m, textinput.Blink
			}
		}
	}
	return m, nil
}

func (m Model) updateCustomMode(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			switch m.customFocus {
			case customFocusInput:
				// Move to save button on enter from input
				m.customFocus = customFocusSave
				m.customInput.Blur()
				return m, nil
			case customFocusSave:
				hex := m.customInput.Value()
				if isValidHex(hex) {
					return m, selectCmd(hex)
				}
				// Show error when Save clicked with invalid hex
				m.showCustomError = true
				return m, nil
			case customFocusCancel:
				m.inCustomMode = false
				m.customFocus = customFocusInput
				m.showCustomError = false
				return m, nil
			}
		case "esc":
			m.inCustomMode = false
			m.customFocus = customFocusInput
			m.showCustomError = false
			m.customInput.Blur()
			return m, nil
		case "tab", "down", "ctrl+n":
			if m.customFocus < customFocusCancel {
				m.customFocus++
				m.customInput.Blur()
			} else {
				// Cycle back to input
				m.customFocus = customFocusInput
				m.customInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "j":
			// j only navigates when not focused on text input
			if m.customFocus != customFocusInput {
				if m.customFocus < customFocusCancel {
					m.customFocus++
				} else {
					m.customFocus = customFocusInput
					m.customInput.Focus()
					return m, textinput.Blink
				}
			}
			// Fall through to text input update when focused
		case "shift+tab", "up", "ctrl+p":
			if m.customFocus > customFocusInput {
				m.customFocus--
				if m.customFocus == customFocusInput {
					m.customInput.Focus()
					return m, textinput.Blink
				}
			} else {
				// Cycle to cancel button
				m.customFocus = customFocusCancel
				m.customInput.Blur()
			}
			return m, nil
		case "k":
			// k only navigates when not focused on text input
			if m.customFocus != customFocusInput {
				if m.customFocus > customFocusInput {
					m.customFocus--
					if m.customFocus == customFocusInput {
						m.customInput.Focus()
						return m, textinput.Blink
					}
				} else {
					m.customFocus = customFocusCancel
					m.customInput.Blur()
				}
			}
			// Fall through to text input update when focused
		case "h", "left":
			if m.customFocus == customFocusCancel {
				m.customFocus = customFocusSave
			}
			return m, nil
		case "l", "right":
			if m.customFocus == customFocusSave {
				m.customFocus = customFocusCancel
			}
			return m, nil
		}
	}

	// Only update text input when focused on it
	if m.customFocus == customFocusInput {
		var cmd tea.Cmd
		m.customInput, cmd = m.customInput.Update(msg)
		// Clear error when valid hex is typed
		if m.showCustomError && isValidHex(m.customInput.Value()) {
			m.showCustomError = false
		}
		return m, cmd
	}
	return m, nil
}

// View renders the picker box.
func (m Model) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.OverlayTitleColor).
		PaddingLeft(1)

	width := m.boxWidth
	if width == 0 {
		width = 30
	}

	var content strings.Builder

	if m.inCustomMode {
		content.WriteString(titleStyle.Render("Custom Color"))
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(styles.OverlayBorderColor).Render(strings.Repeat("─", width)))
		content.WriteString("\n")

		// Input with inline preview swatch, wrapped in bordered section
		hex := m.customInput.Value()
		inputLine := m.customInput.View()
		if isValidHex(hex) {
			previewSwatch := lipgloss.NewStyle().
				Background(lipgloss.Color(hex)).
				Render("    ")
			inputLine = inputLine + "  " + previewSwatch
		}
		inputSection := styles.RenderFormSection([]string{inputLine}, "Hex", "#RRGGBB", width-2, m.customFocus == customFocusInput, styles.BorderHighlightFocusColor)
		content.WriteString(lipgloss.NewStyle().PaddingLeft(1).Render(inputSection))
		content.WriteString("\n")

		// Error message (only shown after clicking Save with invalid hex)
		if m.showCustomError {
			content.WriteString(lipgloss.NewStyle().PaddingLeft(1).Foreground(styles.StatusErrorColor).Render("Invalid hex format"))
			content.WriteString("\n")
		}

		// Always show buttons
		content.WriteString("\n")
		saveStyle := styles.PrimaryButtonStyle
		if m.customFocus == customFocusSave {
			saveStyle = styles.PrimaryButtonFocusedStyle
		}
		cancelStyle := styles.SecondaryButtonStyle
		if m.customFocus == customFocusCancel {
			cancelStyle = styles.PrimaryButtonFocusedStyle
		}
		saveBtn := saveStyle.Render("Save")
		cancelBtn := cancelStyle.Render("Cancel")
		content.WriteString(lipgloss.NewStyle().PaddingLeft(1).Render(saveBtn + "  " + cancelBtn))
	} else {
		content.WriteString(titleStyle.Render("Select Color"))
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Foreground(styles.OverlayBorderColor).Render(strings.Repeat("─", width)))
		content.WriteString("\n")

		// Find the max rows across all columns
		maxRows := 0
		for _, col := range m.columns {
			if len(col) > maxRows {
				maxRows = len(col)
			}
		}

		// Render columns side by side
		columnWidth := 16 // Compact column width for 4 columns
		var columnViews []string
		for colIdx, presets := range m.columns {
			var colContent strings.Builder
			isActiveColumn := colIdx == m.column
			for rowIdx := 0; rowIdx < maxRows; rowIdx++ {
				if rowIdx < len(presets) {
					preset := presets[rowIdx]
					swatch := lipgloss.NewStyle().
						Background(lipgloss.Color(preset.Hex)).
						Render("  ")

					var line string
					if isActiveColumn && rowIdx == m.selected {
						// Selected: white bold ">", then swatch and name
						line = styles.SelectionIndicatorStyle.Render(">") + swatch + " " + preset.Name
					} else {
						// Not selected: space prefix
						line = " " + swatch + " " + preset.Name
					}
					colContent.WriteString(lipgloss.NewStyle().Width(columnWidth).Render(line))
				} else {
					// Empty row to maintain alignment
					colContent.WriteString(strings.Repeat(" ", columnWidth))
				}
				colContent.WriteString("\n")
			}
			columnViews = append(columnViews, colContent.String())
		}

		// Join columns horizontally
		content.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, columnViews...))

		if m.customEnabled {
			content.WriteString("\n")
			content.WriteString(lipgloss.NewStyle().PaddingLeft(1).Foreground(styles.TextPrimaryColor).Render("'c' custom  h/l column"))
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.OverlayBorderColor).
		Width(width)

	return boxStyle.Render(content.String())
}

// Overlay renders the picker on top of a background view.
func (m Model) Overlay(background string) string {
	pickerBox := m.View()

	if background == "" {
		return lipgloss.Place(
			m.viewportWidth, m.viewportHeight,
			lipgloss.Center, lipgloss.Center,
			pickerBox,
		)
	}

	return overlay.Place(overlay.Config{
		Width:    m.viewportWidth,
		Height:   m.viewportHeight,
		Position: overlay.Center,
	}, pickerBox, background)
}

// selectCmd returns a command that sends a SelectMsg.
func selectCmd(hex string) tea.Cmd {
	return func() tea.Msg {
		return SelectMsg{Hex: hex}
	}
}

// cancelCmd returns a command that sends a CancelMsg.
func cancelCmd() tea.Cmd {
	return func() tea.Msg {
		return CancelMsg{}
	}
}

// isValidHex checks if a string is a valid hex color.
func isValidHex(s string) bool {
	matched, _ := regexp.MatchString(`^#[0-9A-Fa-f]{6}$`, s)
	return matched
}
