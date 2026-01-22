package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistration_Getters(t *testing.T) {
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md").
		Node("propose", "Propose", "v1-proposal.md").
		Build()
	require.NoError(t, err)

	reg := newRegistration("workflow", "planning-standard", "v1", "Standard Planning Workflow", "Three-phase workflow: Research, Propose, Plan", "", "", "", chain, nil, nil, SourceBuiltIn)

	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "planning-standard", reg.Key())
	require.Equal(t, "v1", reg.Version())
	require.Equal(t, "Standard Planning Workflow", reg.Name())
	require.Equal(t, "Three-phase workflow: Research, Propose, Plan", reg.Description())
	require.Len(t, reg.DAG().Nodes(), 2)
}

func TestRegistration_EmptyFields(t *testing.T) {
	chain, err := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Build()
	require.NoError(t, err)

	// Registration allows empty name/description - validation is in builder
	reg := newRegistration("workflow", "simple", "v1", "", "", "", "", "", chain, nil, nil, SourceBuiltIn)

	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "simple", reg.Key())
	require.Equal(t, "v1", reg.Version())
	require.Equal(t, "", reg.Name())
	require.Equal(t, "", reg.Description())
	require.Len(t, reg.DAG().Nodes(), 1)
}

func TestRegistration_Template(t *testing.T) {
	chain, err := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Build()
	require.NoError(t, err)

	reg := newRegistration("workflow", "test", "v1", "Test", "Desc", "v1-epic-template.md", "", "", chain, nil, nil, SourceBuiltIn)
	require.Equal(t, "v1-epic-template.md", reg.Template())

	// Empty template
	regNoTemplate := newRegistration("workflow", "test2", "v1", "Test", "Desc", "", "", "", chain, nil, nil, SourceBuiltIn)
	require.Equal(t, "", regNoTemplate.Template())
}

func TestRegistration_Instructions_ReturnsValue(t *testing.T) {
	chain, err := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Build()
	require.NoError(t, err)

	reg := newRegistration("workflow", "test", "v1", "Test", "Desc", "", "epic_driven.md", "", chain, nil, nil, SourceBuiltIn)
	require.Equal(t, "epic_driven.md", reg.Instructions())

	// Empty instructions is allowed at domain level
	regNoInstructions := newRegistration("workflow", "test2", "v1", "Test", "Desc", "", "", "", chain, nil, nil, SourceBuiltIn)
	require.Equal(t, "", regNoInstructions.Instructions())
}

func TestRegistration_DAGAccess(t *testing.T) {
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md").
		Node("propose", "Propose", "v1-proposal.md").
		Node("plan", "Plan", "v1-plan.md").
		Build()
	require.NoError(t, err)

	reg := newRegistration("workflow", "planning-standard", "v1", "Standard", "Description", "", "", "", chain, nil, nil, SourceBuiltIn)

	nodes := reg.DAG().Nodes()
	require.Len(t, nodes, 3)
	require.Equal(t, "research", nodes[0].Key())
	require.Equal(t, "Research", nodes[0].Name())
	require.Equal(t, "v1-research.md", nodes[0].Template())
	require.Equal(t, "propose", nodes[1].Key())
	require.Equal(t, "plan", nodes[2].Key())
}

// Source tests

func TestSource_String(t *testing.T) {
	tests := []struct {
		source   Source
		expected string
	}{
		{SourceBuiltIn, "built-in"},
		{SourceUser, "user"},
		{Source(99), "unknown"}, // Test unknown value
	}

	for _, tc := range tests {
		require.Equal(t, tc.expected, tc.source.String())
	}
}

func TestRegistration_Source(t *testing.T) {
	chain, err := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Build()
	require.NoError(t, err)

	// Test SourceBuiltIn
	regBuiltIn := newRegistration("workflow", "test", "v1", "Test", "Desc", "", "", "", chain, nil, nil, SourceBuiltIn)
	require.Equal(t, SourceBuiltIn, regBuiltIn.Source())

	// Test SourceUser
	regUser := newRegistration("workflow", "test2", "v1", "Test", "Desc", "", "", "", chain, nil, nil, SourceUser)
	require.Equal(t, SourceUser, regUser.Source())
}

func TestSource_DefaultValue(t *testing.T) {
	// Zero value of Source should be SourceBuiltIn (iota starts at 0)
	var s Source
	require.Equal(t, SourceBuiltIn, s)
	require.Equal(t, "built-in", s.String())
}
