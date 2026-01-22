package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper to create a simple chain for tests
func testChain(t *testing.T, nodes ...string) *Chain {
	t.Helper()
	builder := NewChain()
	for i := 0; i < len(nodes); i += 3 {
		builder.Node(nodes[i], nodes[i+1], nodes[i+2])
	}
	chain, err := builder.Build()
	require.NoError(t, err)
	return chain
}

func TestNewBuilder(t *testing.T) {
	builder := NewBuilder("workflow")
	require.NotNil(t, builder)
}

func TestBuilder_Build_Success(t *testing.T) {
	chain := testChain(t, "research", "Research", "v1-research.md")

	reg, err := NewBuilder("workflow").
		Key("planning-standard").
		Version("v1").
		Name("Standard Planning Workflow").
		Description("Three-phase workflow").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "planning-standard", reg.Key())
	require.Equal(t, "v1", reg.Version())
	require.Equal(t, "Standard Planning Workflow", reg.Name())
	require.Equal(t, "Three-phase workflow", reg.Description())
	require.Len(t, reg.DAG().Nodes(), 1)
}

func TestBuilder_Build_OptionalFields(t *testing.T) {
	// Name and description are optional
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("minimal").
		Version("v1").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	require.Equal(t, "minimal", reg.Key())
	require.Equal(t, "", reg.Name())
	require.Equal(t, "", reg.Description())
}

func TestBuilder_Build_MultipleChainItems(t *testing.T) {
	chain := testChain(t,
		"research", "Research", "v1-research.md",
		"propose", "Propose", "v1-proposal.md",
		"plan", "Plan", "v1-plan.md",
	)

	reg, err := NewBuilder("workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	nodes := reg.DAG().Nodes()
	require.Len(t, nodes, 3)
	require.Equal(t, "research", nodes[0].Key())
	require.Equal(t, "Research", nodes[0].Name())
	require.Equal(t, "propose", nodes[1].Key())
	require.Equal(t, "Propose", nodes[1].Name())
	require.Equal(t, "plan", nodes[2].Key())
	require.Equal(t, "Plan", nodes[2].Name())
}

func TestBuilder_Build_EmptyType(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("").
		Key("key").
		Version("v1").
		SetChain(chain).
		Build()

	require.Nil(t, reg)
	require.ErrorIs(t, err, ErrEmptyNamespace)
}

func TestBuilder_Build_EmptyKey(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Version("v1").
		SetChain(chain).
		Build()

	require.Nil(t, reg)
	require.ErrorIs(t, err, ErrEmptyKey)
}

func TestBuilder_Build_EmptyVersion(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("key").
		SetChain(chain).
		Build()

	require.Nil(t, reg)
	require.ErrorIs(t, err, ErrEmptyVersion)
}

func TestBuilder_Build_EmptyChain(t *testing.T) {
	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		Build()

	require.Nil(t, reg)
	require.ErrorIs(t, err, ErrEmptyChain)
}

func TestBuilder_FluentChaining(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	// Verify methods return the builder for chaining
	builder := NewBuilder("workflow")
	result := builder.
		Key("key").
		Version("v1").
		Name("name").
		Description("desc").
		SetChain(chain)
	require.Same(t, builder, result)
}

func TestBuilder_SetChain(t *testing.T) {
	// Build a Chain with dependencies
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
		).
		Build()
	require.NoError(t, err)

	// Use SetChain in Builder
	reg, err := NewBuilder("workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	require.NotNil(t, reg.DAG())
	require.Equal(t, 2, len(reg.DAG().Nodes()))
}

func TestBuilder_SetChain_FluentChaining(t *testing.T) {
	chain, _ := NewChain().
		Node("x", "X", "x.md").
		Build()

	builder := NewBuilder("workflow")
	result := builder.SetChain(chain)
	require.Same(t, builder, result)
}

// Registration.DAG() Tests

func TestRegistration_DAG_ReturnsChain(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
		).
		Build()

	reg, _ := NewBuilder("workflow").
		Key("test").
		Version("v1").
		SetChain(chain).
		Build()

	dag := reg.DAG()
	require.NotNil(t, dag)
	require.Len(t, dag.Nodes(), 2)

	// Verify DAG has full dependency information
	deps := dag.DependenciesOf("propose")
	require.Len(t, deps, 1)
	require.Equal(t, "research", deps[0].Key())
}

func TestRegistration_Chain_BackwardCompat(t *testing.T) {
	// Chain() returns []ChainItem from the DAG
	chain, _ := NewChain().
		Node("a", "A", "a.md").
		Node("b", "B", "b.md").
		Node("c", "C", "c.md").
		Build()

	reg, _ := NewBuilder("workflow").
		Key("test").
		Version("v1").
		SetChain(chain).
		Build()

	nodes := reg.DAG().Nodes()
	require.Len(t, nodes, 3)
	require.Equal(t, "a", nodes[0].Key())
	require.Equal(t, "A", nodes[0].Name())
	require.Equal(t, "a.md", nodes[0].Template())
}

// Labels Tests

func TestBuilder_Labels(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Labels("lang:go", "category:workflow").
		Build()

	require.NoError(t, err)
	require.Equal(t, []string{"lang:go", "category:workflow"}, reg.Labels())
}

func TestBuilder_Labels_Empty(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	// Empty labels slice is valid
	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Labels().
		Build()

	require.NoError(t, err)
	require.Empty(t, reg.Labels())
}

func TestBuilder_Labels_FluentChaining(t *testing.T) {
	builder := NewBuilder("workflow")
	result := builder.Labels("lang:go")
	require.Same(t, builder, result)
}

// Instructions Tests

func TestBuilder_Instructions_SetsField(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Instructions("epic_driven.md").
		Build()

	require.NoError(t, err)
	require.Equal(t, "epic_driven.md", reg.Instructions())
}

func TestBuilder_Instructions_EmptyAllowed(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	// Empty instructions is valid at domain level (validation happens in application layer)
	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Instructions("").
		Build()

	require.NoError(t, err)
	require.Equal(t, "", reg.Instructions())
}

func TestBuilder_Instructions_FluentChaining(t *testing.T) {
	builder := NewBuilder("workflow")
	result := builder.Instructions("epic_driven.md")
	require.Same(t, builder, result)
}

func TestBuilder_Build_IncludesInstructions(t *testing.T) {
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("planning-standard").
		Version("v1").
		Name("Standard Planning").
		Description("A workflow").
		Template("v1-epic.md").
		Instructions("epic_driven.md").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	require.Equal(t, "v1-epic.md", reg.Template())
	require.Equal(t, "epic_driven.md", reg.Instructions())
}

// Source Tests

func TestBuilder_Source(t *testing.T) {
	// Verify Source() method sets the source field
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Source(SourceUser).
		Build()

	require.NoError(t, err)
	require.Equal(t, SourceUser, reg.Source())
}

func TestBuilder_DefaultSource(t *testing.T) {
	// Verify omitting Source() defaults to SourceBuiltIn
	chain := testChain(t, "step", "Step", "step.md")

	reg, err := NewBuilder("workflow").
		Key("key").
		Version("v1").
		SetChain(chain).
		Build()

	require.NoError(t, err)
	require.Equal(t, SourceBuiltIn, reg.Source())
}

func TestBuilder_Build_WithSource(t *testing.T) {
	// Verify built registration has correct source for both values
	chain := testChain(t, "step", "Step", "step.md")

	// Test with SourceBuiltIn explicitly
	regBuiltIn, err := NewBuilder("workflow").
		Key("built-in-key").
		Version("v1").
		SetChain(chain).
		Source(SourceBuiltIn).
		Build()

	require.NoError(t, err)
	require.Equal(t, SourceBuiltIn, regBuiltIn.Source())
	require.Equal(t, "built-in", regBuiltIn.Source().String())

	// Test with SourceUser
	regUser, err := NewBuilder("workflow").
		Key("user-key").
		Version("v1").
		SetChain(chain).
		Source(SourceUser).
		Build()

	require.NoError(t, err)
	require.Equal(t, SourceUser, regUser.Source())
	require.Equal(t, "user", regUser.Source().String())
}

func TestBuilder_Source_FluentChaining(t *testing.T) {
	builder := NewBuilder("workflow")
	result := builder.Source(SourceUser)
	require.Same(t, builder, result)
}
