package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantFM      frontmatter
		wantErr     bool
		errContains string
	}{
		{
			name: "valid frontmatter with all fields",
			content: `---
name: "Test Workflow"
description: "A test description"
category: "Testing"
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "A test description",
				Category:    "Testing",
			},
		},
		{
			name: "valid frontmatter with workers field",
			content: `---
name: "Test Workflow"
description: "A test description"
category: "Testing"
workers: 4
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "A test description",
				Category:    "Testing",
				Workers:     4,
			},
		},
		{
			name: "workers defaults to zero when omitted",
			content: `---
name: "Test Workflow"
description: "No workers field"
---

# Content here
`,
			wantFM: frontmatter{
				Name:        "Test Workflow",
				Description: "No workers field",
				Workers:     0,
			},
		},
		{
			name: "valid frontmatter without category",
			content: `---
name: "Simple Workflow"
description: "No category"
---

# Content
`,
			wantFM: frontmatter{
				Name:        "Simple Workflow",
				Description: "No category",
				Category:    "",
			},
		},
		{
			name: "valid frontmatter without description",
			content: `---
name: "Minimal"
---

# Content
`,
			wantFM: frontmatter{
				Name: "Minimal",
			},
		},
		{
			name:        "missing opening delimiter",
			content:     `name: "Test"`,
			wantErr:     true,
			errContains: "does not start with frontmatter delimiter",
		},
		{
			name: "missing closing delimiter",
			content: `---
name: "Test"
`,
			wantErr:     true,
			errContains: "no closing frontmatter delimiter found",
		},
		{
			name: "missing required name field",
			content: `---
description: "Has description but no name"
---

# Content
`,
			wantErr:     true,
			errContains: "missing required field: name",
		},
		{
			name: "invalid YAML syntax",
			content: `---
name: "Test
description: broken
---

# Content
`,
			wantErr:     true,
			errContains: "parsing YAML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, err := parseFrontmatter(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFM, fm)
		})
	}
}

func TestParseWorkflow(t *testing.T) {
	content := `---
name: "Technical Debate"
description: "Multi-perspective debate format"
category: "Analysis"
---

# Technical Debate Format

This is the content of the debate format.
`
	wf, err := parseWorkflow(content, "debate.md", SourceBuiltIn)
	require.NoError(t, err)

	assert.Equal(t, "debate", wf.ID)
	assert.Equal(t, "Technical Debate", wf.Name)
	assert.Equal(t, "Multi-perspective debate format", wf.Description)
	assert.Equal(t, "Analysis", wf.Category)
	assert.Equal(t, 0, wf.Workers) // Default when not specified
	assert.Equal(t, content, wf.Content)
	assert.Equal(t, SourceBuiltIn, wf.Source)
	assert.Empty(t, wf.FilePath)
}

func TestParseWorkflowWithWorkers(t *testing.T) {
	content := `---
name: "Technical Debate"
description: "Multi-perspective debate format"
category: "Analysis"
workers: 4
---

# Technical Debate Format

This is the content of the debate format.
`
	wf, err := parseWorkflow(content, "debate.md", SourceBuiltIn)
	require.NoError(t, err)

	assert.Equal(t, "debate", wf.ID)
	assert.Equal(t, "Technical Debate", wf.Name)
	assert.Equal(t, "Multi-perspective debate format", wf.Description)
	assert.Equal(t, "Analysis", wf.Category)
	assert.Equal(t, 4, wf.Workers) // Explicit worker count
	assert.Equal(t, content, wf.Content)
	assert.Equal(t, SourceBuiltIn, wf.Source)
	assert.Empty(t, wf.FilePath)
}

func TestParseWorkflowFile(t *testing.T) {
	content := `---
name: "Custom Workflow"
description: "User-defined workflow"
---

# Custom Content
`
	wf, err := ParseWorkflowFile(content, "custom.md", "/home/user/.perles/workflows/custom.md", SourceUser)
	require.NoError(t, err)

	assert.Equal(t, "custom", wf.ID)
	assert.Equal(t, "Custom Workflow", wf.Name)
	assert.Equal(t, "User-defined workflow", wf.Description)
	assert.Equal(t, SourceUser, wf.Source)
	assert.Equal(t, "/home/user/.perles/workflows/custom.md", wf.FilePath)
}

func TestLoadBuiltinWorkflows(t *testing.T) {
	workflows, err := LoadBuiltinWorkflows()
	require.NoError(t, err)

	// Should load at least our two built-in templates
	assert.GreaterOrEqual(t, len(workflows), 2, "expected at least 2 built-in workflows")

	// Check that we have both debate and research_proposal
	var foundDebate, foundResearch bool
	for _, wf := range workflows {
		switch wf.ID {
		case "debate":
			foundDebate = true
			assert.Equal(t, "Technical Debate", wf.Name)
			assert.Equal(t, SourceBuiltIn, wf.Source)
			assert.NotEmpty(t, wf.Description)
			assert.NotEmpty(t, wf.Content)
		case "research_proposal":
			foundResearch = true
			assert.Equal(t, "Research Proposal", wf.Name)
			assert.Equal(t, SourceBuiltIn, wf.Source)
			assert.NotEmpty(t, wf.Description)
			assert.NotEmpty(t, wf.Content)
		}
	}

	assert.True(t, foundDebate, "expected to find debate workflow")
	assert.True(t, foundResearch, "expected to find research_proposal workflow")
}

func TestLoadBuiltinWorkflowsHaveCorrectWorkerCounts(t *testing.T) {
	workflows, err := LoadBuiltinWorkflows()
	require.NoError(t, err)

	// Map workflow IDs to expected worker counts
	expectedWorkers := map[string]int{
		"debate":            4,
		"cook":              4,
		"quick_plan":        4,
		"research_proposal": 4,
		"research_to_tasks": 0, // Default when not specified
	}

	for _, wf := range workflows {
		expected, hasExpectation := expectedWorkers[wf.ID]
		if hasExpectation {
			assert.Equal(t, expected, wf.Workers, "workflow %s should have %d workers", wf.ID, expected)
		}
	}
}

func TestLoadUserWorkflowsFromDir(t *testing.T) {
	t.Run("non-existent directory returns empty slice", func(t *testing.T) {
		workflows, err := LoadUserWorkflowsFromDir("/non/existent/path")
		require.NoError(t, err)
		assert.Empty(t, workflows)
	})

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		dir := t.TempDir()
		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		assert.Empty(t, workflows)
	})

	t.Run("loads valid workflow files", func(t *testing.T) {
		dir := t.TempDir()

		// Create a valid workflow file
		content := `---
name: "Test Workflow"
description: "A test workflow"
category: "Testing"
---

# Test Content
`
		err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(content), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)

		wf := workflows[0]
		assert.Equal(t, "test", wf.ID)
		assert.Equal(t, "Test Workflow", wf.Name)
		assert.Equal(t, "A test workflow", wf.Description)
		assert.Equal(t, "Testing", wf.Category)
		assert.Equal(t, content, wf.Content)
		assert.Equal(t, SourceUser, wf.Source)
		assert.Equal(t, filepath.Join(dir, "test.md"), wf.FilePath)
	})

	t.Run("loads multiple workflow files", func(t *testing.T) {
		dir := t.TempDir()

		// Create two valid workflow files
		content1 := `---
name: "First Workflow"
description: "First"
---

# First
`
		content2 := `---
name: "Second Workflow"
description: "Second"
---

# Second
`
		err := os.WriteFile(filepath.Join(dir, "first.md"), []byte(content1), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "second.md"), []byte(content2), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		assert.Len(t, workflows, 2)

		// Verify both were loaded (order may vary)
		ids := make(map[string]bool)
		for _, wf := range workflows {
			ids[wf.ID] = true
			assert.Equal(t, SourceUser, wf.Source)
		}
		assert.True(t, ids["first"])
		assert.True(t, ids["second"])
	})

	t.Run("skips invalid frontmatter", func(t *testing.T) {
		dir := t.TempDir()

		// Create a valid and an invalid workflow file
		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		invalidContent := `no frontmatter here`

		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "invalid.md"), []byte(invalidContent), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("skips non-md files", func(t *testing.T) {
		dir := t.TempDir()

		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a workflow"), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("skips subdirectories", func(t *testing.T) {
		dir := t.TempDir()

		validContent := `---
name: "Valid Workflow"
description: "Valid"
---

# Valid
`
		err := os.WriteFile(filepath.Join(dir, "valid.md"), []byte(validContent), 0o644)
		require.NoError(t, err)

		// Create a subdirectory with an md file (should be ignored)
		subdir := filepath.Join(dir, "subdir")
		err = os.MkdirAll(subdir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(subdir, "nested.md"), []byte(validContent), 0o644)
		require.NoError(t, err)

		workflows, err := LoadUserWorkflowsFromDir(dir)
		require.NoError(t, err)
		require.Len(t, workflows, 1)
		assert.Equal(t, "valid", workflows[0].ID)
	})

	t.Run("errors if path is a file", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "notadir")
		err := os.WriteFile(filePath, []byte("content"), 0o644)
		require.NoError(t, err)

		_, err = LoadUserWorkflowsFromDir(filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestUserWorkflowDir(t *testing.T) {
	dir := UserWorkflowDir()
	// Should return a path ending in .perles/workflows
	assert.Contains(t, dir, ".perles")
	assert.True(t, filepath.IsAbs(dir))
}
