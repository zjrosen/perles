package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Helper to create a simple chain for tests
func mkChain(t *testing.T, nodes ...string) *Chain {
	t.Helper()
	builder := NewChain()
	for i := 0; i < len(nodes); i += 3 {
		builder.Node(nodes[i], nodes[i+1], nodes[i+2])
	}
	chain, err := builder.Build()
	require.NoError(t, err)
	return chain
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
	require.Empty(t, reg.List())
}

func TestRegistry_Add(t *testing.T) {
	registry := NewRegistry()
	registration, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()

	err := registry.Add(registration)

	require.NoError(t, err)
	require.Len(t, registry.List(), 1)
}

func TestRegistry_Add_NilRegistration(t *testing.T) {
	registry := NewRegistry()

	err := registry.Add(nil)

	require.ErrorIs(t, err, ErrNilRegistration)
	require.Empty(t, registry.List())
}

func TestRegistry_Add_DuplicateKey(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	err := registry.Add(reg1)
	require.NoError(t, err)

	// Same type and key, different version
	reg2, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v2").
		SetChain(mkChain(t, "research", "Research", "v2-research.md")).
		Build()
	err = registry.Add(reg2)

	require.ErrorIs(t, err, ErrDuplicateKey)
}

func TestRegistry_Add_DifferentKeySameType(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	reg2, _ := NewBuilder("spec-workflow").
		Key("planning-simple").
		Version("v1").
		SetChain(mkChain(t, "plan", "Plan", "v1-plan.md")).
		Build()

	err1 := registry.Add(reg1)
	err2 := registry.Add(reg2)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Len(t, registry.List(), 2)
}

func TestRegistry_Add_SameKeyDifferentType(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	reg2, _ := NewBuilder("spec-template").
		Key("standard").
		Version("v1").
		SetChain(mkChain(t, "create", "Create", "v1-create.md")).
		Build()

	err1 := registry.Add(reg1)
	err2 := registry.Add(reg2)

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Len(t, registry.List(), 2)
}

func TestRegistry_GetByNamespace_SingleMatch(t *testing.T) {
	registry := NewRegistry()

	reg, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	registry.Add(reg)

	result := registry.GetByNamespace("spec-workflow")

	require.Len(t, result, 1)
	require.Equal(t, "planning-standard", result[0].Key())
}

func TestRegistry_GetByNamespace_MultipleMatches(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	reg2, _ := NewBuilder("spec-workflow").
		Key("planning-simple").
		Version("v1").
		SetChain(mkChain(t, "plan", "Plan", "v1-plan.md")).
		Build()

	registry.Add(reg1)
	registry.Add(reg2)

	result := registry.GetByNamespace("spec-workflow")

	require.Len(t, result, 2)
}

func TestRegistry_GetByNamespace_NotFound(t *testing.T) {
	registry := NewRegistry()

	result := registry.GetByNamespace("nonexistent")

	require.Empty(t, result)
}

func TestRegistry_GetByKey(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		Name("Standard").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	reg2, _ := NewBuilder("spec-workflow").
		Key("planning-simple").
		Version("v1").
		Name("Simple").
		SetChain(mkChain(t, "plan", "Plan", "v1-plan.md")).
		Build()

	registry.Add(reg1)
	registry.Add(reg2)

	result, err := registry.GetByKey("spec-workflow", "planning-simple")

	require.NoError(t, err)
	require.Equal(t, "planning-simple", result.Key())
	require.Equal(t, "Simple", result.Name())
}

func TestRegistry_GetByKey_NotFound(t *testing.T) {
	registry := NewRegistry()

	reg, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	registry.Add(reg)

	result, err := registry.GetByKey("spec-workflow", "nonexistent")

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRegistry_GetByKey_WrongType(t *testing.T) {
	registry := NewRegistry()

	reg, _ := NewBuilder("spec-workflow").
		Key("standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	registry.Add(reg)

	result, err := registry.GetByKey("spec-template", "standard")

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("planning-standard").
		Version("v1").
		SetChain(mkChain(t, "research", "Research", "v1-research.md")).
		Build()
	reg2, _ := NewBuilder("spec-template").
		Key("create").
		Version("v1").
		SetChain(mkChain(t, "create", "Create", "v1-create.md")).
		Build()

	registry.Add(reg1)
	registry.Add(reg2)

	list := registry.List()

	require.Len(t, list, 2)
}

// GetByLabels Tests

func TestRegistry_GetByLabels_SingleLabel(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("go-workflow").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:go").
		Build()
	reg2, _ := NewBuilder("spec-workflow").
		Key("python-workflow").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:python").
		Build()

	registry.Add(reg1)
	registry.Add(reg2)

	result := registry.GetByLabels("lang:go")

	require.Len(t, result, 1)
	require.Equal(t, "go-workflow", result[0].Key())
}

func TestRegistry_GetByLabels_MultipleLabels_ANDLogic(t *testing.T) {
	registry := NewRegistry()

	// Has both labels
	reg1, _ := NewBuilder("spec-workflow").
		Key("go-guidelines").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:go", "category:guidelines").
		Build()
	// Has only one label
	reg2, _ := NewBuilder("spec-workflow").
		Key("go-workflow").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:go").
		Build()
	// Has neither
	reg3, _ := NewBuilder("spec-workflow").
		Key("python-workflow").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:python").
		Build()

	registry.Add(reg1)
	registry.Add(reg2)
	registry.Add(reg3)

	// AND logic: must have BOTH labels
	result := registry.GetByLabels("lang:go", "category:guidelines")

	require.Len(t, result, 1)
	require.Equal(t, "go-guidelines", result[0].Key())
}

func TestRegistry_GetByLabels_NoLabels_ReturnsAll(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("workflow-1").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:go").
		Build()
	reg2, _ := NewBuilder("spec-workflow").
		Key("workflow-2").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Build()

	registry.Add(reg1)
	registry.Add(reg2)

	// No labels passed = return all
	result := registry.GetByLabels()

	require.Len(t, result, 2)
}

func TestRegistry_GetByLabels_NoMatches(t *testing.T) {
	registry := NewRegistry()

	reg1, _ := NewBuilder("spec-workflow").
		Key("go-workflow").
		Version("v1").
		SetChain(mkChain(t, "step", "Step", "step.md")).
		Labels("lang:go").
		Build()

	registry.Add(reg1)

	result := registry.GetByLabels("lang:python")

	require.Empty(t, result)
}
