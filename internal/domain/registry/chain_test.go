package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewChain(t *testing.T) {
	builder := NewChain()
	require.NotNil(t, builder)
}

func TestChainBuilder_Node(t *testing.T) {
	builder := NewChain().
		Node("research", "Research", "v1-research.md")

	chain, err := builder.Build()
	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 1)

	node := chain.Nodes()[0]
	require.Equal(t, "research", node.Key())
	require.Equal(t, "Research", node.Name())
	require.Equal(t, "v1-research.md", node.Template())
}

func TestChainBuilder_MultipleNodes(t *testing.T) {
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md").
		Node("propose", "Propose", "v1-proposal.md").
		Node("plan", "Plan", "v1-plan.md").
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 3)

	require.Equal(t, "research", chain.Nodes()[0].Key())
	require.Equal(t, "propose", chain.Nodes()[1].Key())
	require.Equal(t, "plan", chain.Nodes()[2].Key())
}

func TestChainBuilder_Inputs(t *testing.T) {
	// Need a producer node for the input artifact
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 2)

	node := chain.Nodes()[1] // propose is second
	require.Len(t, node.Inputs(), 1)
	require.Equal(t, "research.md", node.Inputs()[0].Filename())
}

func TestChainBuilder_Outputs(t *testing.T) {
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 1)

	node := chain.Nodes()[0]
	require.Len(t, node.Outputs(), 1)
	require.Equal(t, "research.md", node.Outputs()[0].Filename())
}

func TestChainBuilder_After(t *testing.T) {
	// Need the referenced node to exist
	chain, err := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Node("notify", "Notify", "v1-notify.md",
			After("plan"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 2)

	node := chain.Nodes()[1] // notify is second
	require.Len(t, node.After(), 1)
	require.Equal(t, "plan", node.After()[0])
}

func TestChainBuilder_FluentChaining(t *testing.T) {
	// Test the complete functional options API
	chain, err := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
			Outputs("proposal.md"),
		).
		Node("plan", "Plan", "v1-plan.md",
			Inputs("proposal.md"),
			Outputs("plan.md", "tasks.json"),
		).
		Node("notify", "Notify", "v1-notify.md",
			After("plan"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 4)

	// Verify research node
	research := chain.Nodes()[0]
	require.Equal(t, "research", research.Key())
	require.Empty(t, research.Inputs())
	require.Len(t, research.Outputs(), 1)
	require.Equal(t, "research.md", research.Outputs()[0].Filename())

	// Verify propose node
	propose := chain.Nodes()[1]
	require.Equal(t, "propose", propose.Key())
	require.Len(t, propose.Inputs(), 1)
	require.Len(t, propose.Outputs(), 1)

	// Verify plan node
	plan := chain.Nodes()[2]
	require.Equal(t, "plan", plan.Key())
	require.Len(t, plan.Inputs(), 1)
	require.Len(t, plan.Outputs(), 2)

	// Verify notify node
	notify := chain.Nodes()[3]
	require.Equal(t, "notify", notify.Key())
	require.Empty(t, notify.Inputs())
	require.Empty(t, notify.Outputs())
	require.Len(t, notify.After(), 1)
}

func TestChain_RootNodes_SingleRoot(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
		).
		Build()

	roots := chain.RootNodes()
	require.Len(t, roots, 1)
	require.Equal(t, "research", roots[0].Key())
}

func TestChain_RootNodes_MultipleRoots(t *testing.T) {
	// Parallel starting nodes
	chain, _ := NewChain().
		Node("tech-research", "Tech Research", "v1-tech.md",
			Outputs("tech.md"),
		).
		Node("user-research", "User Research", "v1-user.md",
			Outputs("user.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("tech.md", "user.md"),
		).
		Build()

	roots := chain.RootNodes()
	require.Len(t, roots, 2)

	keys := []string{roots[0].Key(), roots[1].Key()}
	require.Contains(t, keys, "tech-research")
	require.Contains(t, keys, "user-research")
}

func TestChain_RootNodes_NoRoots(t *testing.T) {
	// All nodes have dependencies - create a valid chain where all nodes depend on each other
	chain, err := NewChain().
		Node("a", "A", "a.md",
			Outputs("a.md"),
		).
		Node("b", "B", "b.md",
			Inputs("a.md"),
		).
		Build()

	require.NoError(t, err)
	// Only "a" is a root, "b" depends on "a"
	roots := chain.RootNodes()
	require.Len(t, roots, 1)
	require.Equal(t, "a", roots[0].Key())
}

func TestChain_RootNodes_AllRoots(t *testing.T) {
	// No dependencies between nodes
	chain, _ := NewChain().
		Node("a", "A", "a.md").
		Node("b", "B", "b.md").
		Node("c", "C", "c.md").
		Build()

	roots := chain.RootNodes()
	require.Len(t, roots, 3)
}

func TestChain_DependenciesOf_FromInputs(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
		).
		Build()

	deps := chain.DependenciesOf("propose")
	require.Len(t, deps, 1)
	require.Equal(t, "research", deps[0].Key())
}

func TestChain_DependenciesOf_FromAfter(t *testing.T) {
	chain, _ := NewChain().
		Node("plan", "Plan", "v1-plan.md").
		Node("notify", "Notify", "v1-notify.md",
			After("plan"),
		).
		Build()

	deps := chain.DependenciesOf("notify")
	require.Len(t, deps, 1)
	require.Equal(t, "plan", deps[0].Key())
}

func TestChain_DependenciesOf_Combined(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("setup", "Setup", "v1-setup.md").
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
			After("setup"),
		).
		Build()

	deps := chain.DependenciesOf("propose")
	require.Len(t, deps, 2)

	keys := []string{deps[0].Key(), deps[1].Key()}
	require.Contains(t, keys, "research")
	require.Contains(t, keys, "setup")
}

func TestChain_DependenciesOf_NoDuplicates(t *testing.T) {
	// If same node produces artifact AND is in After list, should only appear once
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("research.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"),
			After("research"), // Same as artifact producer
		).
		Build()

	deps := chain.DependenciesOf("propose")
	require.Len(t, deps, 1)
	require.Equal(t, "research", deps[0].Key())
}

func TestChain_DependenciesOf_NoDependencies(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md").
		Build()

	deps := chain.DependenciesOf("research")
	require.Empty(t, deps)
}

func TestChain_DependenciesOf_UnknownNode(t *testing.T) {
	chain, _ := NewChain().
		Node("research", "Research", "v1-research.md").
		Build()

	deps := chain.DependenciesOf("unknown")
	require.Nil(t, deps)
}

// TestChain_DependenciesOf_MissingProducer is now a validation error test
// See TestChain_Build_DetectsDanglingInput

// TestChain_DependenciesOf_UnknownAfterRef is now a validation error test
// See TestChain_Build_DetectsUnknownAfter

func TestChain_Immutability(t *testing.T) {
	// Chain should be immutable after Build()
	builder := NewChain().
		Node("research", "Research", "v1-research.md")

	chain1, _ := builder.Build()

	// Adding more nodes after Build shouldn't affect the built chain
	builder.Node("propose", "Propose", "v1-proposal.md")
	chain2, _ := builder.Build()

	require.Len(t, chain1.Nodes(), 1) // Still 1 node
	require.Len(t, chain2.Nodes(), 2) // Has both nodes
}

// Validation Tests

func TestChain_Build_DetectsEmptyChain(t *testing.T) {
	_, err := NewChain().Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrChainEmpty)
}

func TestChain_Build_DetectsDuplicateNode(t *testing.T) {
	_, err := NewChain().
		Node("research", "Research", "v1-research.md").
		Node("research", "Research Again", "v1-research2.md"). // Duplicate key
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDuplicateNode)
	require.Contains(t, err.Error(), "research")
}

func TestChain_Build_DetectsDuplicateOutput(t *testing.T) {
	_, err := NewChain().
		Node("research", "Research", "v1-research.md",
			Outputs("output.md"),
		).
		Node("propose", "Propose", "v1-proposal.md",
			Outputs("output.md"), // Same output as research
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDuplicateOutput)
	require.Contains(t, err.Error(), "output.md")
	require.Contains(t, err.Error(), "research")
	require.Contains(t, err.Error(), "propose")
}

func TestChain_Build_DetectsDanglingInput(t *testing.T) {
	_, err := NewChain().
		Node("propose", "Propose", "v1-proposal.md",
			Inputs("research.md"), // No producer
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrDanglingInput)
	require.Contains(t, err.Error(), "research.md")
	require.Contains(t, err.Error(), "propose")
}

func TestChain_Build_DetectsUnknownAfter(t *testing.T) {
	_, err := NewChain().
		Node("notify", "Notify", "v1-notify.md",
			After("unknown"), // Node doesn't exist
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnknownAfterNode)
	require.Contains(t, err.Error(), "unknown")
	require.Contains(t, err.Error(), "notify")
}

func TestChain_Build_DetectsCycle_Direct(t *testing.T) {
	// A depends on B, B depends on A
	_, err := NewChain().
		Node("a", "A", "a.md",
			Outputs("a.md"),
			Inputs("b.md"), // A needs B's output
		).
		Node("b", "B", "b.md",
			Outputs("b.md"),
			Inputs("a.md"), // B needs A's output
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCycleDetected)
}

func TestChain_Build_DetectsCycle_Indirect(t *testing.T) {
	// A -> B -> C -> A (indirect cycle)
	_, err := NewChain().
		Node("a", "A", "a.md",
			Outputs("a.md"),
			Inputs("c.md"),
		).
		Node("b", "B", "b.md",
			Outputs("b.md"),
			Inputs("a.md"),
		).
		Node("c", "C", "c.md",
			Outputs("c.md"),
			Inputs("b.md"),
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCycleDetected)
}

func TestChain_Build_DetectsCycle_ViaAfter(t *testing.T) {
	// A depends on B via After, B depends on A via After
	_, err := NewChain().
		Node("a", "A", "a.md",
			After("b"),
		).
		Node("b", "B", "b.md",
			After("a"),
		).
		Build()

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCycleDetected)
}

func TestChain_Build_NoCycle_Linear(t *testing.T) {
	// Valid linear chain: A -> B -> C
	chain, err := NewChain().
		Node("a", "A", "a.md",
			Outputs("a.md"),
		).
		Node("b", "B", "b.md",
			Inputs("a.md"),
			Outputs("b.md"),
		).
		Node("c", "C", "c.md",
			Inputs("b.md"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 3)
}

func TestChain_Build_NoCycle_Diamond(t *testing.T) {
	// Valid diamond: A -> B, A -> C, B -> D, C -> D
	chain, err := NewChain().
		Node("a", "A", "a.md",
			Outputs("a.md"),
		).
		Node("b", "B", "b.md",
			Inputs("a.md"),
			Outputs("b.md"),
		).
		Node("c", "C", "c.md",
			Inputs("a.md"),
			Outputs("c.md"),
		).
		Node("d", "D", "d.md",
			Inputs("b.md", "c.md"),
		).
		Build()

	require.NoError(t, err)
	require.Len(t, chain.Nodes(), 4)

	// Verify root
	roots := chain.RootNodes()
	require.Len(t, roots, 1)
	require.Equal(t, "a", roots[0].Key())

	// Verify D depends on both B and C
	deps := chain.DependenciesOf("d")
	require.Len(t, deps, 2)
}
