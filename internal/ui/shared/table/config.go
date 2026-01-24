// Package table provides a config-driven shared table component for rendering
// consistent, styled tables across the application.
//
// The table component is a pure render component with external state management.
// Callers pass column configurations (with required Render callbacks), row data,
// and dimensions. The component handles bordered pane wrapping, header rendering,
// cell truncation, and selection highlighting.
//
// Quick Start:
//
//	cfg := table.TableConfig{
//	    Columns: []table.ColumnConfig{
//	        {Key: "id", Header: "#", Width: 3, Render: func(row any, _ string, w int, _ bool) string {
//	            return fmt.Sprintf("%*d", w, row.(*MyRow).ID)
//	        }},
//	        {Key: "name", Header: "Name", MinWidth: 10, Render: func(row any, _ string, w int, _ bool) string {
//	            return styles.TruncateString(row.(*MyRow).Name, w)
//	        }},
//	    },
//	    ShowHeader: true,
//	    ShowBorder: true,
//	}
//	tbl := table.New(cfg).SetRows(rows).SetSize(80, 20)
//	view := tbl.ViewWithSelection(selectedIndex)
//
// Column Types:
//
// While the Type field provides semantic information about column content, all
// columns require explicit Render callbacks in v1. The Type field is primarily
// for documentation and potential future default rendering.
//
// Selection:
//
// The table component does not manage selection state internally. Use View() for
// rendering without selection, or ViewWithSelection(index) for highlighting a
// specific row. This allows easy integration with existing selection/filtering logic.
package table

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ColumnType identifies the semantic type of column content.
// This provides documentation and enables potential future default rendering.
type ColumnType int

const (
	// ColumnTypeText is plain text content (default).
	ColumnTypeText ColumnType = iota

	// ColumnTypeIcon is for status icons or emoji.
	ColumnTypeIcon

	// ColumnTypeDate is for date/time values.
	ColumnTypeDate

	// ColumnTypeNumber is for numeric values (typically right-aligned).
	ColumnTypeNumber

	// ColumnTypeCustom indicates fully custom rendering via callback.
	ColumnTypeCustom
)

// ColumnConfig defines a single table column.
//
// Common fields:
//   - Key: Unique identifier for the column (used in Render callback and documentation)
//   - Header: Column header text displayed in the header row
//   - Type: Semantic type of column content (default: ColumnTypeText)
//
// Width configuration:
//   - Width: Fixed width in characters (0 = flex/auto)
//   - MinWidth: Minimum width for flex columns (0 = no minimum beyond 3)
//   - MaxWidth: Maximum width for flex columns (0 = no limit)
//
// Rendering:
//   - Align: Text alignment (lipgloss.Left, lipgloss.Center, lipgloss.Right)
//   - Render: Required callback for cell rendering
//
// The Render callback signature is:
//
//	func(row any, key string, width int, selected bool) string
//
// Where:
//   - row: The row data (caller performs type assertion)
//   - key: The column key (useful for generic render functions)
//   - width: Available width for the cell content
//   - selected: Whether this row is currently selected
type ColumnConfig struct {
	Key      string     // Unique identifier for data extraction and documentation
	Header   string     // Column header text
	Type     ColumnType // Semantic type (default: ColumnTypeText)
	Width    int        // Fixed width (0 = flex/auto)
	MinWidth int        // Minimum width for flex columns
	MaxWidth int        // Maximum width (0 = no limit)
	Align    lipgloss.Position

	// HideBelow hides this column when total table width falls below this threshold.
	// Set to 0 to always show the column (default behavior).
	// Useful for responsive layouts where less important columns are hidden when space is limited.
	HideBelow int

	// Render provides cell content rendering.
	// Required for all columns - the table does not support reflection-based value extraction.
	// Signature: (rowData any, colKey string, width int, selected bool) -> string
	Render func(row any, key string, width int, selected bool) string
}

// TableConfig defines the complete table configuration.
//
// Required fields:
//   - Columns: At least one column configuration with Render callback
//
// Display options:
//   - ShowHeader: Show header row (default: true when using defaults)
//   - ShowBorder: Wrap in bordered pane (default: true when using defaults)
//   - Title: Optional title for bordered pane
//   - EmptyMessage: Message when no rows (default: "No data")
//
// Selection:
//   - Selectable: Enable row selection highlighting (for documentation)
//
// Callbacks:
//   - OnSelect: Produces message when row is selected (j/k navigation)
//   - OnActivate: Produces message when row is activated (Enter key)
//
// Style overrides (optional - uses defaults from styles package):
//   - HeaderStyle: Header row style
//   - RowStyle: Normal row style
//   - SelectedStyle: Selected row background
//   - BorderColor: Border color override
type TableConfig struct {
	Columns      []ColumnConfig // Column definitions (required, at least one)
	ShowHeader   bool           // Show header row
	ShowBorder   bool           // Wrap in bordered pane
	Title        string         // Optional title for bordered pane
	EmptyMessage string         // Message when no rows (default: "No data")

	// Scrolling configuration
	Scrollable bool // Enable scrolling with sticky header (requires Update() calls)

	// Selection configuration (for documentation - state is external)
	Selectable bool

	// Callbacks for custom message types (optional)
	OnSelect   func(index int, row any) tea.Msg // Row selection callback
	OnActivate func(index int, row any) tea.Msg // Enter key callback

	// RowZoneID returns a bubblezone zone ID for a row (optional).
	// When set, each row is wrapped with zone.Mark() for mouse click detection.
	// Signature: (index int, row any) -> zoneID string
	RowZoneID func(index int, row any) string

	// Style overrides (optional - uses defaults from styles package)
	HeaderStyle        lipgloss.Style         // Header row style
	RowStyle           lipgloss.Style         // Normal row style
	SelectedStyle      lipgloss.Style         // Selected row background
	BorderColor        lipgloss.TerminalColor // Border color override
	Focused            bool                   // Whether the table has focus (affects border color)
	FocusedBorderColor lipgloss.TerminalColor // Border color when focused
}

// ValidateConfig validates the table configuration.
// Returns an error if:
//   - Columns is empty
//   - Any column has a nil Render callback
func ValidateConfig(cfg TableConfig) error {
	if len(cfg.Columns) == 0 {
		return errors.New("table config: at least one column is required")
	}

	for i, col := range cfg.Columns {
		if col.Render == nil {
			if col.Key != "" {
				return errors.New("table config: column \"" + col.Key + "\" has nil Render callback")
			}
			return errors.New("table config: column " + string(rune('0'+i)) + " has nil Render callback")
		}
	}

	return nil
}
