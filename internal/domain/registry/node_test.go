package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewNode(t *testing.T) {
	node := NewNode("research", "Research", "v1-research.md")

	require.Equal(t, "research", node.Key())
	require.Equal(t, "Research", node.Name())
	require.Equal(t, "v1-research.md", node.Template())
	require.Empty(t, node.Inputs())
	require.Empty(t, node.Outputs())
	require.Empty(t, node.After())
}

func TestNode_Getters(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		nodeName string
		template string
	}{
		{
			name:     "standard node",
			key:      "research",
			nodeName: "Research",
			template: "v1-research.md",
		},
		{
			name:     "node with spaces in name",
			key:      "research-propose",
			nodeName: "Research & Propose",
			template: "v1-research-proposal.md",
		},
		{
			name:     "node with empty template",
			key:      "empty",
			nodeName: "Empty",
			template: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNode(tt.key, tt.nodeName, tt.template)
			require.Equal(t, tt.key, node.Key())
			require.Equal(t, tt.nodeName, node.Name())
			require.Equal(t, tt.template, node.Template())
		})
	}
}

func TestNode_WithInputs(t *testing.T) {
	node := NewNode("propose", "Propose", "v1-proposal.md")
	researchArtifact := NewArtifact("research.md")

	result := node.WithInputs(researchArtifact)

	// Returns same node for fluent chaining
	require.Same(t, node, result)
	require.Len(t, node.Inputs(), 1)
	require.Equal(t, "research.md", node.Inputs()[0].Filename())
}

func TestNode_WithInputs_Multiple(t *testing.T) {
	node := NewNode("plan", "Plan", "v1-plan.md")
	research := NewArtifact("research.md")
	proposal := NewArtifact("proposal.md")

	node.WithInputs(research, proposal)

	require.Len(t, node.Inputs(), 2)
	require.Equal(t, "research.md", node.Inputs()[0].Filename())
	require.Equal(t, "proposal.md", node.Inputs()[1].Filename())
}

func TestNode_WithInputs_Appends(t *testing.T) {
	node := NewNode("merge", "Merge", "v1-merge.md")
	node.WithInputs(NewArtifact("a.md"))
	node.WithInputs(NewArtifact("b.md"))

	require.Len(t, node.Inputs(), 2)
	require.Equal(t, "a.md", node.Inputs()[0].Filename())
	require.Equal(t, "b.md", node.Inputs()[1].Filename())
}

func TestNode_WithOutputs(t *testing.T) {
	node := NewNode("research", "Research", "v1-research.md")
	output := NewArtifact("research.md")

	result := node.WithOutputs(output)

	// Returns same node for fluent chaining
	require.Same(t, node, result)
	require.Len(t, node.Outputs(), 1)
	require.Equal(t, "research.md", node.Outputs()[0].Filename())
}

func TestNode_WithOutputs_Multiple(t *testing.T) {
	node := NewNode("plan", "Plan", "v1-plan.md")
	plan := NewArtifact("plan.md")
	tasks := NewArtifact("tasks.json")

	node.WithOutputs(plan, tasks)

	require.Len(t, node.Outputs(), 2)
	require.Equal(t, "plan.md", node.Outputs()[0].Filename())
	require.Equal(t, "tasks.json", node.Outputs()[1].Filename())
}

func TestNode_WithAfter(t *testing.T) {
	node := NewNode("notify", "Notify", "v1-notify.md")

	result := node.WithAfter("plan")

	// Returns same node for fluent chaining
	require.Same(t, node, result)
	require.Len(t, node.After(), 1)
	require.Equal(t, "plan", node.After()[0])
}

func TestNode_WithAfter_Multiple(t *testing.T) {
	node := NewNode("notify", "Notify", "v1-notify.md")

	node.WithAfter("plan", "review")

	require.Len(t, node.After(), 2)
	require.Equal(t, "plan", node.After()[0])
	require.Equal(t, "review", node.After()[1])
}

func TestNode_WithAfter_Appends(t *testing.T) {
	node := NewNode("final", "Final", "v1-final.md")
	node.WithAfter("a")
	node.WithAfter("b")

	require.Len(t, node.After(), 2)
	require.Equal(t, "a", node.After()[0])
	require.Equal(t, "b", node.After()[1])
}

func TestNode_WithAssignee(t *testing.T) {
	node := NewNode("research", "Research", "v1-research.md")

	result := node.WithAssignee("research-lead")

	// Returns same node for fluent chaining
	require.Same(t, node, result)
	require.Equal(t, "research-lead", node.Assignee())
}

func TestNode_Assignee_Empty(t *testing.T) {
	node := NewNode("research", "Research", "v1-research.md")

	// Default assignee is empty
	require.Equal(t, "", node.Assignee())
}

func TestNode_FluentChaining(t *testing.T) {
	// Test the full fluent API
	node := NewNode("propose", "Propose", "v1-proposal.md").
		WithInputs(NewArtifact("research.md")).
		WithOutputs(NewArtifact("proposal.md")).
		WithAfter("setup").
		WithAssignee("architect")

	require.Equal(t, "propose", node.Key())
	require.Equal(t, "Propose", node.Name())
	require.Equal(t, "v1-proposal.md", node.Template())
	require.Len(t, node.Inputs(), 1)
	require.Equal(t, "research.md", node.Inputs()[0].Filename())
	require.Len(t, node.Outputs(), 1)
	require.Equal(t, "proposal.md", node.Outputs()[0].Filename())
	require.Len(t, node.After(), 1)
	require.Equal(t, "setup", node.After()[0])
	require.Equal(t, "architect", node.Assignee())
}

func TestNode_ComplexWorkflow(t *testing.T) {
	// Simulate the standard planning workflow nodes
	research := NewNode("research", "Research", "v1-research.md").
		WithOutputs(NewArtifact("research.md"))

	propose := NewNode("propose", "Propose", "v1-proposal.md").
		WithInputs(NewArtifact("research.md")).
		WithOutputs(NewArtifact("proposal.md"))

	plan := NewNode("plan", "Plan", "v1-plan.md").
		WithInputs(NewArtifact("proposal.md")).
		WithOutputs(NewArtifact("plan.md"), NewArtifact("tasks.json"))

	notify := NewNode("notify", "Notify", "v1-notify.md").
		WithAfter("plan")

	// Verify research node
	require.Empty(t, research.Inputs())
	require.Len(t, research.Outputs(), 1)
	require.Empty(t, research.After())

	// Verify propose node
	require.Len(t, propose.Inputs(), 1)
	require.Len(t, propose.Outputs(), 1)
	require.Empty(t, propose.After())

	// Verify plan node
	require.Len(t, plan.Inputs(), 1)
	require.Len(t, plan.Outputs(), 2)
	require.Empty(t, plan.After())

	// Verify notify node
	require.Empty(t, notify.Inputs())
	require.Empty(t, notify.Outputs())
	require.Len(t, notify.After(), 1)
}
