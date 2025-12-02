// Package config provides configuration types, defaults, and persistence for perles.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SaveViews updates the views configuration in the config file.
// This preserves comments and formatting in other sections by using yaml.Node.
func SaveViews(configPath string, views []ViewConfig) error {
	// Read existing file content
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading config: %w", err)
	}

	// Parse into yaml.Node to preserve comments
	var doc yaml.Node
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}

	// Build the new views node
	viewsNode, err := buildViewsNode(views)
	if err != nil {
		return fmt.Errorf("building views node: %w", err)
	}

	// Update or create the views section
	if doc.Kind == 0 {
		// Empty or new file - create document structure
		doc = yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{
					Kind: yaml.MappingNode,
					Content: []*yaml.Node{
						{Kind: yaml.ScalarNode, Value: "views"},
						viewsNode,
					},
				},
			},
		}
	} else if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root := doc.Content[0]
		if root.Kind == yaml.MappingNode {
			// Find and replace views key, or append it
			found := false
			for i := 0; i < len(root.Content)-1; i += 2 {
				if root.Content[i].Value == "views" {
					root.Content[i+1] = viewsNode
					found = true
					break
				}
			}
			if !found {
				root.Content = append(root.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "views"},
					viewsNode,
				)
			}
		}
	}

	// Marshal back to YAML
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&doc); err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	_ = encoder.Close()

	// Write atomically (write to temp, then rename)
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".perles.yaml.tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tempPath := temp.Name()

	if _, err := temp.Write(buf.Bytes()); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tempPath, configPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// SaveColumns updates the columns in the first view (backward compatibility).
// This preserves comments and formatting in other sections by using yaml.Node.
func SaveColumns(configPath string, columns []ColumnConfig) error {
	return SaveColumnsForView(configPath, 0, columns, nil)
}

// SaveColumnsForView updates the columns for a specific view.
// If allViews is nil, it creates a single Default view with the columns.
// If allViews is provided, it updates the columns at viewIndex within those views.
func SaveColumnsForView(configPath string, viewIndex int, columns []ColumnConfig, allViews []ViewConfig) error {
	var views []ViewConfig

	if len(allViews) == 0 {
		// Backward compatibility: wrap in a single Default view
		views = []ViewConfig{
			{
				Name:    "Default",
				Columns: columns,
			},
		}
	} else {
		if viewIndex < 0 || viewIndex >= len(allViews) {
			return fmt.Errorf("view index %d out of range (have %d views)", viewIndex, len(allViews))
		}
		// Copy views and update the target view's columns
		views = make([]ViewConfig, len(allViews))
		copy(views, allViews)
		views[viewIndex].Columns = columns
	}

	return SaveViews(configPath, views)
}

// buildViewsNode creates a yaml.Node representing the views array.
func buildViewsNode(views []ViewConfig) (*yaml.Node, error) {
	node := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: make([]*yaml.Node, 0, len(views)),
	}

	for _, view := range views {
		viewNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: make([]*yaml.Node, 0),
		}

		// Add view name
		viewNode.Content = append(viewNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: view.Name},
		)

		// Add columns
		columnsNode, err := buildColumnsNode(view.Columns)
		if err != nil {
			return nil, err
		}
		viewNode.Content = append(viewNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "columns"},
			columnsNode,
		)

		node.Content = append(node.Content, viewNode)
	}

	return node, nil
}

// buildColumnsNode creates a yaml.Node representing the columns array.
func buildColumnsNode(columns []ColumnConfig) (*yaml.Node, error) {
	node := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: make([]*yaml.Node, 0, len(columns)),
	}

	for _, col := range columns {
		colNode := &yaml.Node{
			Kind:    yaml.MappingNode,
			Content: make([]*yaml.Node, 0),
		}

		// Always include name
		colNode.Content = append(colNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: col.Name},
		)

		// Always include query
		colNode.Content = append(colNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "query"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: col.Query},
		)

		if col.Color != "" {
			colNode.Content = append(colNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "color"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: col.Color},
			)
		}

		node.Content = append(node.Content, colNode)
	}

	return node, nil
}

// UpdateColumn updates a single column in the config and saves.
// Deprecated: Use UpdateColumnInView for multi-view support.
func UpdateColumn(configPath string, index int, newCol ColumnConfig, allCols []ColumnConfig) error {
	return UpdateColumnInView(configPath, 0, index, newCol, allCols, nil)
}

// UpdateColumnInView updates a single column within a specific view.
func UpdateColumnInView(configPath string, viewIndex, colIndex int, newCol ColumnConfig, allCols []ColumnConfig, allViews []ViewConfig) error {
	if colIndex < 0 || colIndex >= len(allCols) {
		return fmt.Errorf("column index %d out of range (have %d columns)", colIndex, len(allCols))
	}

	// Create copy and update
	updated := make([]ColumnConfig, len(allCols))
	copy(updated, allCols)
	updated[colIndex] = newCol

	return SaveColumnsForView(configPath, viewIndex, updated, allViews)
}

// DeleteColumn removes a column from the config and saves.
// Deprecated: Use DeleteColumnInView for multi-view support.
func DeleteColumn(configPath string, index int, allCols []ColumnConfig) error {
	return DeleteColumnInView(configPath, 0, index, allCols, nil)
}

// DeleteColumnInView removes a column from a specific view.
// Deleting the last column is allowed - will result in empty view with empty state UI.
func DeleteColumnInView(configPath string, viewIndex, colIndex int, allCols []ColumnConfig, allViews []ViewConfig) error {
	if colIndex < 0 || colIndex >= len(allCols) {
		return fmt.Errorf("column index %d out of range (have %d columns)", colIndex, len(allCols))
	}

	// Create new slice without the deleted column
	updated := make([]ColumnConfig, 0, len(allCols)-1)
	for i, col := range allCols {
		if i != colIndex {
			updated = append(updated, col)
		}
	}

	return SaveColumnsForView(configPath, viewIndex, updated, allViews)
}

// SwapColumns swaps two columns by index and saves the config.
// Deprecated: Use SwapColumnsInView for multi-view support.
func SwapColumns(configPath string, idxA, idxB int, allCols []ColumnConfig) error {
	return SwapColumnsInView(configPath, 0, idxA, idxB, allCols, nil)
}

// SwapColumnsInView swaps two columns within a specific view.
func SwapColumnsInView(configPath string, viewIndex, idxA, idxB int, allCols []ColumnConfig, allViews []ViewConfig) error {
	if idxA < 0 || idxA >= len(allCols) || idxB < 0 || idxB >= len(allCols) {
		return fmt.Errorf("column index out of range")
	}
	if idxA == idxB {
		return nil // No-op
	}

	updated := make([]ColumnConfig, len(allCols))
	copy(updated, allCols)
	updated[idxA], updated[idxB] = updated[idxB], updated[idxA]

	return SaveColumnsForView(configPath, viewIndex, updated, allViews)
}

// AddColumn inserts a new column at the specified position and saves.
// The column is inserted after the given index (use -1 to insert at beginning).
// Deprecated: Use AddColumnInView for multi-view support.
func AddColumn(configPath string, insertAfterIndex int, newCol ColumnConfig, allCols []ColumnConfig) error {
	return AddColumnInView(configPath, 0, insertAfterIndex, newCol, allCols, nil)
}

// AddView appends a new view to the config and saves it.
func AddView(configPath string, newView ViewConfig, existingViews []ViewConfig) error {
	views := append(existingViews, newView)
	return SaveViews(configPath, views)
}

// DeleteView removes a view at the given index and saves.
// Returns error if viewIndex is out of range or if it's the last view.
func DeleteView(configPath string, viewIndex int, allViews []ViewConfig) error {
	if len(allViews) <= 1 {
		return fmt.Errorf("cannot delete the only view")
	}
	if viewIndex < 0 || viewIndex >= len(allViews) {
		return fmt.Errorf("view index %d out of range (have %d views)", viewIndex, len(allViews))
	}

	// Create new slice without the deleted view
	updated := make([]ViewConfig, 0, len(allViews)-1)
	for i, view := range allViews {
		if i != viewIndex {
			updated = append(updated, view)
		}
	}

	return SaveViews(configPath, updated)
}

// RenameView renames the view at the given index and saves.
// Returns error if viewIndex is out of range or if saving fails.
func RenameView(configPath string, viewIndex int, newName string, allViews []ViewConfig) error {
	if viewIndex < 0 || viewIndex >= len(allViews) {
		return fmt.Errorf("view index %d out of range (have %d views)", viewIndex, len(allViews))
	}

	// Update the view name in the slice
	allViews[viewIndex].Name = newName

	return SaveViews(configPath, allViews)
}

// InsertColumnInView inserts a new column at the specified position within a specific view.
// Position 0 inserts at the beginning of the column list.
func InsertColumnInView(configPath string, viewIndex, position int, newCol ColumnConfig, allViews []ViewConfig) error {
	if viewIndex < 0 || viewIndex >= len(allViews) {
		return fmt.Errorf("view index %d out of range (have %d views)", viewIndex, len(allViews))
	}

	allCols := allViews[viewIndex].Columns

	// Validate position (0 to len(allCols) inclusive)
	if position < 0 || position > len(allCols) {
		return fmt.Errorf("position %d out of range (valid: 0 to %d)", position, len(allCols))
	}

	// Create new slice with space for the new column
	updated := make([]ColumnConfig, 0, len(allCols)+1)

	for i, col := range allCols {
		if i == position {
			updated = append(updated, newCol)
		}
		updated = append(updated, col)
	}

	// Handle insertion at the end
	if position == len(allCols) {
		updated = append(updated, newCol)
	}

	return SaveColumnsForView(configPath, viewIndex, updated, allViews)
}

// AddColumnInView inserts a new column at the specified position within a specific view.
// Deprecated: Use InsertColumnInView which takes position directly.
func AddColumnInView(configPath string, viewIndex, insertAfterIndex int, newCol ColumnConfig, allCols []ColumnConfig, allViews []ViewConfig) error {
	// Validate insertion point
	if insertAfterIndex < -1 || insertAfterIndex >= len(allCols) {
		return fmt.Errorf("insertion index %d out of range (valid: -1 to %d)", insertAfterIndex, len(allCols)-1)
	}

	// Create new slice with space for the new column
	updated := make([]ColumnConfig, 0, len(allCols)+1)

	// Insert at position insertAfterIndex + 1
	insertPos := insertAfterIndex + 1

	for i, col := range allCols {
		if i == insertPos {
			updated = append(updated, newCol)
		}
		updated = append(updated, col)
	}

	// Handle insertion at the end
	if insertPos >= len(allCols) {
		updated = append(updated, newCol)
	}

	return SaveColumnsForView(configPath, viewIndex, updated, allViews)
}
