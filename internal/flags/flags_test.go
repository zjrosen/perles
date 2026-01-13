package flags

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistry_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		registry *Registry
		flag     string
		expected bool
	}{
		{
			name:     "known flag set to true returns true",
			registry: New(map[string]bool{"feature-a": true}),
			flag:     "feature-a",
			expected: true,
		},
		{
			name:     "known flag set to false returns false",
			registry: New(map[string]bool{"feature-b": false}),
			flag:     "feature-b",
			expected: false,
		},
		{
			name:     "unknown flag returns false",
			registry: New(map[string]bool{"feature-a": true}),
			flag:     "unknown-flag",
			expected: false,
		},
		{
			name:     "nil registry returns false",
			registry: nil,
			flag:     "any-flag",
			expected: false,
		},
		{
			name:     "empty registry returns false",
			registry: New(map[string]bool{}),
			flag:     "any-flag",
			expected: false,
		},
		{
			name:     "nil flags map returns false",
			registry: New(nil),
			flag:     "any-flag",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.registry.Enabled(tt.flag)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRegistry_Enabled_MultipleFlags(t *testing.T) {
	r := New(map[string]bool{
		"feature-a": true,
		"feature-b": false,
		"feature-c": true,
	})

	require.True(t, r.Enabled("feature-a"))
	require.False(t, r.Enabled("feature-b"))
	require.True(t, r.Enabled("feature-c"))
	require.False(t, r.Enabled("feature-d")) // unknown
}

func TestRegistry_All(t *testing.T) {
	tests := []struct {
		name     string
		registry *Registry
		expected map[string]bool
	}{
		{
			name:     "returns all flags",
			registry: New(map[string]bool{"a": true, "b": false}),
			expected: map[string]bool{"a": true, "b": false},
		},
		{
			name:     "returns empty map for nil registry",
			registry: nil,
			expected: map[string]bool{},
		},
		{
			name:     "returns empty map for empty registry",
			registry: New(map[string]bool{}),
			expected: map[string]bool{},
		},
		{
			name:     "returns empty map for nil flags",
			registry: New(nil),
			expected: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.registry.All()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRegistry_All_ReturnsDefensiveCopy(t *testing.T) {
	original := map[string]bool{"feature-a": true}
	r := New(original)

	// Get a copy via All()
	copy := r.All()

	// Mutate the copy
	copy["feature-a"] = false
	copy["new-flag"] = true

	// Verify the registry is unaffected
	require.True(t, r.Enabled("feature-a"), "registry should not be affected by copy mutation")
	require.False(t, r.Enabled("new-flag"), "registry should not have new flags from copy mutation")

	// Verify All() returns the original state
	freshCopy := r.All()
	require.Equal(t, map[string]bool{"feature-a": true}, freshCopy)
}

func TestNew_WithNilFlags(t *testing.T) {
	r := New(nil)
	require.NotNil(t, r)
	require.False(t, r.Enabled("any"))
}

func TestNew_WithEmptyFlags(t *testing.T) {
	r := New(map[string]bool{})
	require.NotNil(t, r)
	require.False(t, r.Enabled("any"))
}
