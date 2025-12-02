package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveColumns_CreatesNewFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Test", Query: "status = open", Color: "#FF0000"},
	}

	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Verify content
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: Test")
	assert.Contains(t, string(data), "query: status = open")
	assert.Contains(t, string(data), "color: '#FF0000'")
}

func TestSaveColumns_PreservesOtherConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create initial config with various settings
	initial := `auto_refresh: true
theme:
  highlight: "#FF0000"
  subtle: "#888888"
ui:
  show_counts: false
`
	err := os.WriteFile(configPath, []byte(initial), 0644)
	require.NoError(t, err)

	// Save new columns
	columns := []ColumnConfig{
		{Name: "Ready", Query: "status = open and ready = true", Color: "#00FF00"},
	}
	err = SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Verify other settings preserved
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "auto_refresh: true")
	assert.Contains(t, content, "highlight:")
	assert.Contains(t, content, "show_counts: false")
	// And columns are there
	assert.Contains(t, content, "name: Ready")
}

func TestSaveColumns_Roundtrip(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	original := []ColumnConfig{
		{
			Name:  "Ready",
			Query: "status = open and ready = true",
			Color: "#73F59F",
		},
		{
			Name:  "In Progress",
			Query: "status = in_progress",
			Color: "#54A0FF",
		},
	}

	// Save
	err := SaveColumns(configPath, original)
	require.NoError(t, err)

	// Load back using Viper (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	// Verify roundtrip
	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 2)

	assert.Equal(t, original[0].Name, loaded[0].Columns[0].Name)
	assert.Equal(t, original[0].Query, loaded[0].Columns[0].Query)
	assert.Equal(t, original[0].Color, loaded[0].Columns[0].Color)

	assert.Equal(t, original[1].Name, loaded[0].Columns[1].Name)
	assert.Equal(t, original[1].Query, loaded[0].Columns[1].Query)
}

func TestUpdateColumn(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Blocked", Query: "status = open and blocked = true", Color: "#FF0000"},
		{Name: "Ready", Query: "status = open and ready = true", Color: "#00FF00"},
		{Name: "Done", Query: "status = closed", Color: "#0000FF"},
	}

	// Save initial columns
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Update the middle column
	newCol := ColumnConfig{Name: "Ready to Go", Query: "ready = true", Color: "#AABBCC"}
	err = UpdateColumn(configPath, 1, newCol, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 3)
	assert.Equal(t, "Blocked", loaded[0].Columns[0].Name)
	assert.Equal(t, "Ready to Go", loaded[0].Columns[1].Name)
	assert.Equal(t, "#AABBCC", loaded[0].Columns[1].Color)
	assert.Equal(t, "ready = true", loaded[0].Columns[1].Query)
	assert.Equal(t, "Done", loaded[0].Columns[2].Name)
}

func TestUpdateColumn_OutOfRange(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Only", Query: "status = open"},
	}

	err := UpdateColumn(configPath, 5, ColumnConfig{Name: "New", Query: "status = open"}, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	err = UpdateColumn(configPath, -1, ColumnConfig{Name: "New", Query: "status = open"}, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestDeleteColumn(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Blocked", Query: "blocked = true", Color: "#FF0000"},
		{Name: "Ready", Query: "ready = true", Color: "#00FF00"},
		{Name: "Done", Query: "status = closed", Color: "#0000FF"},
	}

	// Save initial columns
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Delete the middle column (Ready)
	err = DeleteColumn(configPath, 1, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 2)
	assert.Equal(t, "Blocked", loaded[0].Columns[0].Name)
	assert.Equal(t, "Done", loaded[0].Columns[1].Name)
}

func TestDeleteColumn_DeletesLastColumn(t *testing.T) {
	// Deleting the last column is allowed - results in empty view with empty state UI
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{{Name: "Only", Query: "status = open"}}

	err := DeleteColumn(configPath, 0, columns)
	require.NoError(t, err)

	// Verify file was saved with empty columns
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	assert.Empty(t, loaded[0].Columns)
}

func TestDeleteColumn_OutOfRange(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "One", Query: "status = open"},
		{Name: "Two", Query: "status = closed"},
	}

	err := DeleteColumn(configPath, 5, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	err = DeleteColumn(configPath, -1, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestSaveColumns_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create initial file
	initial := []ColumnConfig{{Name: "Initial", Query: "status = open"}}
	err := SaveColumns(configPath, initial)
	require.NoError(t, err)

	// Save again - should work without leaving temp files
	columns := []ColumnConfig{{Name: "Updated", Query: "status = closed"}}
	err = SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Check no temp files left behind
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.False(t, filepath.Ext(entry.Name()) == ".tmp", "temp file left behind: %s", entry.Name())
	}

	// Verify content
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: Updated")
}

func TestSaveColumns_CreatesDirectory(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "subdir", "nested", ".perles.yaml")

	columns := []ColumnConfig{{Name: "Test", Query: "status = open"}}
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(configPath)
	require.NoError(t, err)
}

func TestSaveColumns_OmitsEmptyFields(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Column with minimal fields (name and query are required)
	columns := []ColumnConfig{
		{Name: "Minimal", Query: "status = open"},
	}

	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)

	// Should have name and query
	assert.Contains(t, content, "name: Minimal")
	assert.Contains(t, content, "query: status = open")

	// Should NOT have empty color
	assert.NotContains(t, content, "color:")
}

func TestAddColumn_InsertMiddle(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Blocked", Query: "blocked = true"},
		{Name: "Ready", Query: "ready = true"},
		{Name: "Done", Query: "status = closed"},
	}
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Insert after Ready (index 1) -> should appear at position 2
	newCol := ColumnConfig{Name: "Review", Query: "label = review", Color: "#FF0000"}
	err = AddColumn(configPath, 1, newCol, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 4)
	assert.Equal(t, "Blocked", loaded[0].Columns[0].Name)
	assert.Equal(t, "Ready", loaded[0].Columns[1].Name)
	assert.Equal(t, "Review", loaded[0].Columns[2].Name) // New column
	assert.Equal(t, "#FF0000", loaded[0].Columns[2].Color)
	assert.Equal(t, "Done", loaded[0].Columns[3].Name)
}

func TestAddColumn_InsertAtEnd(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Only", Query: "status = open"},
	}
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Insert after Only (index 0) -> should appear at position 1 (end)
	newCol := ColumnConfig{Name: "Second", Query: "status = closed"}
	err = AddColumn(configPath, 0, newCol, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 2)
	assert.Equal(t, "Only", loaded[0].Columns[0].Name)
	assert.Equal(t, "Second", loaded[0].Columns[1].Name)
}

func TestAddColumn_InsertAtBeginning(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Only", Query: "status = open"},
	}
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Insert at beginning (index -1) -> should appear at position 0
	newCol := ColumnConfig{Name: "First", Query: "blocked = true"}
	err = AddColumn(configPath, -1, newCol, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 2)
	assert.Equal(t, "First", loaded[0].Columns[0].Name)
	assert.Equal(t, "Only", loaded[0].Columns[1].Name)
}

func TestAddColumn_InvalidIndex(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "One", Query: "status = open"},
		{Name: "Two", Query: "status = closed"},
	}

	// Index too high
	err := AddColumn(configPath, 5, ColumnConfig{Name: "New", Query: "blocked = true"}, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Index too low
	err = AddColumn(configPath, -2, ColumnConfig{Name: "New", Query: "blocked = true"}, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestAddColumn_EmptyColumnArray(t *testing.T) {
	// Adding first column to empty view
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Start with empty columns
	columns := []ColumnConfig{}

	// Insert at beginning (index -1) when array is empty
	newCol := ColumnConfig{Name: "First", Query: "status = open", Color: "#00FF00"}
	err := AddColumn(configPath, -1, newCol, columns)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 1)
	assert.Equal(t, "First", loaded[0].Columns[0].Name)
}

func TestSwapColumns(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "Blocked", Query: "blocked = true", Color: "#FF0000"},
		{Name: "Ready", Query: "ready = true", Color: "#00FF00"},
		{Name: "Done", Query: "status = closed", Color: "#0000FF"},
	}

	// Save initial columns
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Swap first and second columns
	err = SwapColumns(configPath, 0, 1, columns)
	require.NoError(t, err)

	// Load and verify (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 3)
	assert.Equal(t, "Ready", loaded[0].Columns[0].Name)
	assert.Equal(t, "Blocked", loaded[0].Columns[1].Name)
	assert.Equal(t, "Done", loaded[0].Columns[2].Name)
}

func TestSwapColumns_SameIndex(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "One", Query: "status = open"},
		{Name: "Two", Query: "status = closed"},
	}

	// Save initial
	err := SaveColumns(configPath, columns)
	require.NoError(t, err)

	// Swap same index should be a no-op
	err = SwapColumns(configPath, 1, 1, columns)
	require.NoError(t, err)

	// Load and verify nothing changed (now stored under views[0].columns)
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 2)
	assert.Equal(t, "One", loaded[0].Columns[0].Name)
	assert.Equal(t, "Two", loaded[0].Columns[1].Name)
}

func TestSwapColumns_OutOfRange(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	columns := []ColumnConfig{
		{Name: "One", Query: "status = open"},
		{Name: "Two", Query: "status = closed"},
	}

	// Index too high
	err := SwapColumns(configPath, 0, 5, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Index negative
	err = SwapColumns(configPath, -1, 1, columns)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestAddView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create existing views
	existingViews := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, existingViews)
	require.NoError(t, err)

	// Add new view
	newView := ViewConfig{
		Name: "Bugs",
		Columns: []ColumnConfig{
			{Name: "All Bugs", Query: "type = bug", Color: "#FF0000"},
		},
	}
	err = AddView(configPath, newView, existingViews)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 2)
	assert.Equal(t, "Default", loaded[0].Name)
	assert.Equal(t, "Bugs", loaded[1].Name)
	require.Len(t, loaded[1].Columns, 1)
	assert.Equal(t, "All Bugs", loaded[1].Columns[0].Name)
	assert.Equal(t, "type = bug", loaded[1].Columns[0].Query)
	assert.Equal(t, "#FF0000", loaded[1].Columns[0].Color)
}

func TestAddView_Empty(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// No existing views
	var existingViews []ViewConfig

	// Add first view
	newView := ViewConfig{
		Name: "First View",
		Columns: []ColumnConfig{
			{Name: "All Issues", Query: "status != closed"},
		},
	}
	err := AddView(configPath, newView, existingViews)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	assert.Equal(t, "First View", loaded[0].Name)
	require.Len(t, loaded[0].Columns, 1)
	assert.Equal(t, "All Issues", loaded[0].Columns[0].Name)
}

func TestDeleteView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create views
	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
		{
			Name: "Bugs",
			Columns: []ColumnConfig{
				{Name: "All Bugs", Query: "type = bug", Color: "#FF0000"},
			},
		},
		{
			Name: "Features",
			Columns: []ColumnConfig{
				{Name: "All Features", Query: "type = feature"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Delete the middle view (Bugs)
	err = DeleteView(configPath, 1, views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 2)
	assert.Equal(t, "Default", loaded[0].Name)
	assert.Equal(t, "Features", loaded[1].Name)
}

func TestDeleteView_FirstView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create two views
	views := []ViewConfig{
		{
			Name: "First",
			Columns: []ColumnConfig{
				{Name: "Col1", Query: "status = open"},
			},
		},
		{
			Name: "Second",
			Columns: []ColumnConfig{
				{Name: "Col2", Query: "status = closed"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Delete the first view (index 0)
	err = DeleteView(configPath, 0, views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	assert.Equal(t, "Second", loaded[0].Name)
}

func TestDeleteView_LastView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create only one view
	views := []ViewConfig{
		{
			Name: "Only",
			Columns: []ColumnConfig{
				{Name: "Col", Query: "status = open"},
			},
		},
	}

	err := DeleteView(configPath, 0, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot delete the only view")
}

func TestDeleteView_OutOfRange(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "One",
			Columns: []ColumnConfig{
				{Name: "Col1", Query: "status = open"},
			},
		},
		{
			Name: "Two",
			Columns: []ColumnConfig{
				{Name: "Col2", Query: "status = closed"},
			},
		},
	}

	// Index too high
	err := DeleteView(configPath, 5, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Index negative
	err = DeleteView(configPath, -1, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

// Tests for InsertColumnInView

func TestInsertColumnInView_InsertsAtPosition0(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Existing1", Query: "status = open"},
				{Name: "Existing2", Query: "status = closed"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Insert at position 0
	newCol := ColumnConfig{Name: "First", Query: "priority = 0", Color: "#FF0000"}
	err = InsertColumnInView(configPath, 0, 0, newCol, views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 3)
	assert.Equal(t, "First", loaded[0].Columns[0].Name)
	assert.Equal(t, "#FF0000", loaded[0].Columns[0].Color)
	assert.Equal(t, "Existing1", loaded[0].Columns[1].Name)
	assert.Equal(t, "Existing2", loaded[0].Columns[2].Name)
}

func TestInsertColumnInView_PreservesExistingColumns(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Blocked", Query: "blocked = true", Color: "#FF0000"},
				{Name: "Ready", Query: "ready = true", Color: "#00FF00"},
				{Name: "Done", Query: "status = closed", Color: "#0000FF"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Insert at position 0
	newCol := ColumnConfig{Name: "New Column", Query: "new = true", Color: "#AABBCC"}
	err = InsertColumnInView(configPath, 0, 0, newCol, views)
	require.NoError(t, err)

	// Load and verify existing columns are unchanged
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 4)

	// Verify original columns preserved with correct data
	assert.Equal(t, "New Column", loaded[0].Columns[0].Name)
	assert.Equal(t, "Blocked", loaded[0].Columns[1].Name)
	assert.Equal(t, "#FF0000", loaded[0].Columns[1].Color)
	assert.Equal(t, "blocked = true", loaded[0].Columns[1].Query)
	assert.Equal(t, "Ready", loaded[0].Columns[2].Name)
	assert.Equal(t, "#00FF00", loaded[0].Columns[2].Color)
	assert.Equal(t, "Done", loaded[0].Columns[3].Name)
	assert.Equal(t, "#0000FF", loaded[0].Columns[3].Color)
}

func TestInsertColumnInView_PreservesOtherConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create initial config with other settings
	initial := `auto_refresh: true
theme:
  highlight: "#FF0000"
views:
  - name: Default
    columns:
      - name: Open
        query: status = open
`
	err := os.WriteFile(configPath, []byte(initial), 0644)
	require.NoError(t, err)

	// Load views to get proper struct
	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}

	// Insert new column
	newCol := ColumnConfig{Name: "New", Query: "new = true"}
	err = InsertColumnInView(configPath, 0, 0, newCol, views)
	require.NoError(t, err)

	// Verify other settings preserved
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, "auto_refresh: true")
	assert.Contains(t, content, "highlight:")
	assert.Contains(t, content, "name: New")
	assert.Contains(t, content, "name: Open")
}

func TestInsertColumnInView_InvalidViewIndex(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}

	newCol := ColumnConfig{Name: "New", Query: "new = true"}

	// Index too high
	err := InsertColumnInView(configPath, 5, 0, newCol, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Index negative
	err = InsertColumnInView(configPath, -1, 0, newCol, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestInsertColumnInView_InvalidPosition(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}

	newCol := ColumnConfig{Name: "New", Query: "new = true"}

	// Position too high (only valid: 0, 1)
	err := InsertColumnInView(configPath, 0, 5, newCol, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Position negative
	err = InsertColumnInView(configPath, 0, -1, newCol, views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}

func TestInsertColumnInView_AtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Insert new column
	newCol := ColumnConfig{Name: "New", Query: "new = true", Color: "#73F59F"}
	err = InsertColumnInView(configPath, 0, 0, newCol, views)
	require.NoError(t, err)

	// Check no temp files left behind
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	for _, entry := range entries {
		name := entry.Name()
		assert.False(t, name != ".perles.yaml" && filepath.Ext(name) == ".tmp",
			"temp file left behind: %s", name)
	}

	// Verify content was written correctly
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "name: New")
}

func TestInsertColumnInView_MultipleViews(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Default Col", Query: "status = open"},
			},
		},
		{
			Name: "Bugs",
			Columns: []ColumnConfig{
				{Name: "Bug Col", Query: "type = bug"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Insert into second view (index 1)
	newCol := ColumnConfig{Name: "New Bug Col", Query: "new = true", Color: "#FF0000"}
	err = InsertColumnInView(configPath, 1, 0, newCol, views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 2)
	// First view should be unchanged
	require.Len(t, loaded[0].Columns, 1)
	assert.Equal(t, "Default Col", loaded[0].Columns[0].Name)
	// Second view should have new column at front
	require.Len(t, loaded[1].Columns, 2)
	assert.Equal(t, "New Bug Col", loaded[1].Columns[0].Name)
	assert.Equal(t, "Bug Col", loaded[1].Columns[1].Name)
}

func TestInsertColumnInView_EmptyColumns(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name:    "Empty View",
			Columns: []ColumnConfig{},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Insert into empty column list
	newCol := ColumnConfig{Name: "First", Query: "status = open", Color: "#00FF00"}
	err = InsertColumnInView(configPath, 0, 0, newCol, views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 1)
	require.Len(t, loaded[0].Columns, 1)
	assert.Equal(t, "First", loaded[0].Columns[0].Name)
}

func TestRenameView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create views
	views := []ViewConfig{
		{
			Name: "Default",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
		{
			Name: "Bugs",
			Columns: []ColumnConfig{
				{Name: "All Bugs", Query: "type = bug"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Rename the second view
	err = RenameView(configPath, 1, "Critical Bugs", views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 2)
	assert.Equal(t, "Default", loaded[0].Name)
	assert.Equal(t, "Critical Bugs", loaded[1].Name)
}

func TestRenameView_FirstView(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	// Create views
	views := []ViewConfig{
		{
			Name: "First",
			Columns: []ColumnConfig{
				{Name: "Col1", Query: "status = open"},
			},
		},
		{
			Name: "Second",
			Columns: []ColumnConfig{
				{Name: "Col2", Query: "status = closed"},
			},
		},
	}

	// Save initial views
	err := SaveViews(configPath, views)
	require.NoError(t, err)

	// Rename the first view
	err = RenameView(configPath, 0, "Renamed First", views)
	require.NoError(t, err)

	// Load and verify
	v := viper.New()
	v.SetConfigFile(configPath)
	err = v.ReadInConfig()
	require.NoError(t, err)

	var loaded []ViewConfig
	err = v.UnmarshalKey("views", &loaded)
	require.NoError(t, err)

	require.Len(t, loaded, 2)
	assert.Equal(t, "Renamed First", loaded[0].Name)
	assert.Equal(t, "Second", loaded[1].Name)
}

func TestRenameView_OutOfRange(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".perles.yaml")

	views := []ViewConfig{
		{
			Name: "One",
			Columns: []ColumnConfig{
				{Name: "Col1", Query: "status = open"},
			},
		},
	}

	// Index too high
	err := RenameView(configPath, 5, "New Name", views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")

	// Index negative
	err = RenameView(configPath, -1, "New Name", views)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "out of range")
}
