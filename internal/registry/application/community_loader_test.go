package registry

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/registry/domain"
)

// validCommunityYAML returns a template.yaml with two workflow registrations for testing.
func validCommunityYAML() string {
	return `registry:
  - namespace: "workflow"
    key: "joke-contest"
    version: "v1"
    name: "Joke Contest"
    description: "A community joke contest workflow"
    labels:
      - "community"
      - "fun"
    nodes:
      - key: "research"
        name: "Research Phase"
        template: "research.md"
        outputs:
          - key: "research"
            file: "research.md"
  - namespace: "workflow"
    key: "code-review"
    version: "v1"
    name: "Code Review"
    description: "A community code review workflow"
    labels:
      - "community"
      - "review"
    nodes:
      - key: "review"
        name: "Review Phase"
        template: "review.md"
        outputs:
          - key: "review"
            file: "review.md"
`
}

func TestLoadCommunityRegistryFromFS_NilSource(t *testing.T) {
	regs, fsys, err := LoadCommunityRegistryFromFS(nil, nil)

	require.NoError(t, err)
	require.Nil(t, regs)
	require.Nil(t, fsys)
}

func TestLoadCommunityRegistryFromFS_EmptyEnabledIDs(t *testing.T) {
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	require.NoError(t, err)
	require.Nil(t, regs)
	require.Nil(t, fsys)
}

func TestLoadCommunityRegistryFromFS_LoadsFilteredRegistrations(t *testing.T) {
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
		"workflows/joke-contest/research.md":   &fstest.MapFile{Data: []byte("# Research")},
		"workflows/joke-contest/review.md":     &fstest.MapFile{Data: []byte("# Review")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"workflow/joke-contest"},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Len(t, regs, 1, "should only return the enabled workflow")

	reg := regs[0]
	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "joke-contest", reg.Key())
	require.Equal(t, registry.SourceCommunity, reg.Source())
}

func TestLoadCommunityRegistryFromFS_UnmatchedIDWarns(t *testing.T) {
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
		"workflows/joke-contest/research.md":   &fstest.MapFile{Data: []byte("# Research")},
		"workflows/joke-contest/review.md":     &fstest.MapFile{Data: []byte("# Review")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"workflow/nonexistent"},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	// Should not error - WARN is logged instead
	require.NoError(t, err)
	require.NotNil(t, fsys, "should return FS even when no IDs match")
	require.Empty(t, regs, "no registrations should match a nonexistent ID")
}

func TestLoadCommunityRegistryFromFS_ZeroRegistrations(t *testing.T) {
	// FS with only a README.md - no template.yaml files
	communityFS := fstest.MapFS{
		"workflows/README.md": &fstest.MapFile{Data: []byte("# Community Workflows")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"workflow/something"},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	// Zero registrations triggers the "no workflow registrations found" error path
	// which should be WARN+skip, never crash
	require.NoError(t, err)
	require.NotNil(t, fsys, "should return FS even with zero registrations")
	require.Nil(t, regs, "should return nil registrations for zero-registration FS")
}

func TestLoadCommunityRegistryFromFS_MalformedYAML(t *testing.T) {
	communityFS := fstest.MapFS{
		"workflows/broken/template.yaml": &fstest.MapFile{Data: []byte(`registry:
  - namespace: broken
    key: [this is invalid yaml
`)},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"workflow/broken"},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	// Malformed YAML should be WARN+skip, never crash
	require.NoError(t, err)
	require.NotNil(t, fsys, "should return FS even with malformed YAML")
	require.Nil(t, regs, "should return nil registrations for malformed YAML")
}

func TestLoadCommunityRegistryFromFS_BareKeyNormalization(t *testing.T) {
	// Bare keys (without namespace/) should be auto-prefixed with "workflow/"
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
		"workflows/joke-contest/research.md":   &fstest.MapFile{Data: []byte("# Research")},
		"workflows/joke-contest/review.md":     &fstest.MapFile{Data: []byte("# Review")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"joke-contest"}, // bare key, no "workflow/" prefix
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Len(t, regs, 1, "bare key should resolve to workflow/joke-contest")

	reg := regs[0]
	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "joke-contest", reg.Key())
	require.Equal(t, registry.SourceCommunity, reg.Source())
}

func TestLoadCommunityRegistryFromFS_MixedBareAndQualifiedKeys(t *testing.T) {
	// Mix of bare keys and fully-qualified keys should both work
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
		"workflows/joke-contest/research.md":   &fstest.MapFile{Data: []byte("# Research")},
		"workflows/joke-contest/review.md":     &fstest.MapFile{Data: []byte("# Review")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"joke-contest", "workflow/code-review"}, // mixed formats
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Len(t, regs, 2, "both bare and qualified keys should resolve")

	keys := make(map[string]bool)
	for _, r := range regs {
		keys[r.Key()] = true
	}
	require.True(t, keys["joke-contest"], "bare key joke-contest should resolve")
	require.True(t, keys["code-review"], "qualified key workflow/code-review should resolve")
}

func TestNormalizeCommunityID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"joke-contest", "workflow/joke-contest"},
		{"workflow/joke-contest", "workflow/joke-contest"},
		{"custom-ns/my-workflow", "custom-ns/my-workflow"},
		{"simple", "workflow/simple"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.expected, normalizeCommunityID(tt.input))
		})
	}
}

func TestLoadCommunityRegistryFromFS_AllIDsMatch(t *testing.T) {
	communityFS := fstest.MapFS{
		"workflows/joke-contest/template.yaml": &fstest.MapFile{Data: []byte(validCommunityYAML())},
		"workflows/joke-contest/research.md":   &fstest.MapFile{Data: []byte("# Research")},
		"workflows/joke-contest/review.md":     &fstest.MapFile{Data: []byte("# Review")},
	}

	source := &CommunitySource{
		FS:         communityFS,
		EnabledIDs: []string{"workflow/joke-contest", "workflow/code-review"},
	}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Len(t, regs, 2, "both enabled IDs should be returned")

	// Verify both registrations are present
	keys := make(map[string]bool)
	for _, r := range regs {
		keys[r.Key()] = true
		require.Equal(t, registry.SourceCommunity, r.Source())
	}
	require.True(t, keys["joke-contest"], "joke-contest should be in results")
	require.True(t, keys["code-review"], "code-review should be in results")
}

func TestLoadCommunityRegistryFromFS_FallsBackToBuiltinSharedTemplates(t *testing.T) {
	// Community workflow references a shared template that only exists in the built-in FS
	communityFS := fstest.MapFS{
		"workflows/reviewed/template.yaml": &fstest.MapFile{Data: []byte(`registry:
  - namespace: "workflow"
    key: "reviewed"
    version: "v1"
    name: "Reviewed"
    description: "Community workflow with a human review gate"
    nodes:
      - key: "work"
        name: "Work"
        template: "work.md"
        assignee: "worker-1"
      - key: "review"
        name: "Human Review"
        template: "v1-human-review.md"
        assignee: "human"
        after:
          - "work"
`)},
		"workflows/reviewed/work.md": &fstest.MapFile{Data: []byte("# Work")},
	}
	builtinFS := fstest.MapFS{
		"workflows/v1-epic-instructions.md": &fstest.MapFile{Data: []byte("# Built-in Epic Instructions")},
		"workflows/v1-human-review.md":      &fstest.MapFile{Data: []byte("# Built-in Human Review")},
	}
	source := &CommunitySource{FS: communityFS, EnabledIDs: []string{"reviewed"}}

	regs, fsys, err := LoadCommunityRegistryFromFS(source, builtinFS)

	require.NoError(t, err)
	require.Len(t, regs, 1)
	reg := regs[0]
	require.Equal(t, registry.SourceCommunity, reg.Source())
	require.Equal(t, "workflows/v1-epic-instructions.md", reg.SystemPrompt())

	content, err := fs.ReadFile(fsys, reg.SystemPrompt())
	require.NoError(t, err)
	require.Equal(t, "# Built-in Epic Instructions", string(content))

	var reviewTemplate string
	for _, node := range reg.DAG().Nodes() {
		if node.Key() == "review" {
			reviewTemplate = node.Template()
		}
	}
	require.Equal(t, "workflows/v1-human-review.md", reviewTemplate)
	content, err = fs.ReadFile(fsys, reviewTemplate)
	require.NoError(t, err)
	require.Equal(t, "# Built-in Human Review", string(content))

	// Without the built-in fallback, the missing shared templates fail the load
	regs, _, err = LoadCommunityRegistryFromFS(source, nil)
	require.NoError(t, err)
	require.Nil(t, regs)
}
