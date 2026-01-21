package registry

import (
	"errors"
	"fmt"
)

// Validation errors returned by ChainBuilder.Build()
var (
	ErrChainEmpty       = errors.New("chain must have at least one node")
	ErrDuplicateNode    = errors.New("duplicate node key")
	ErrDuplicateOutput  = errors.New("duplicate artifact output")
	ErrDanglingInput    = errors.New("artifact required but not produced")
	ErrUnknownAfterNode = errors.New("unknown node in After reference")
	ErrCycleDetected    = errors.New("cycle detected in dependency graph")
)

// NodeOption configures a Node during chain building.
type NodeOption func(*Node)

// Inputs adds input artifacts to a node.
func Inputs(filenames ...string) NodeOption {
	return func(n *Node) {
		for _, f := range filenames {
			n.WithInputs(NewArtifact(f))
		}
	}
}

// Outputs adds output artifacts to a node.
func Outputs(filenames ...string) NodeOption {
	return func(n *Node) {
		for _, f := range filenames {
			n.WithOutputs(NewArtifact(f))
		}
	}
}

// After adds ordering dependencies to a node.
func After(keys ...string) NodeOption {
	return func(n *Node) {
		n.WithAfter(keys...)
	}
}

// Assignee sets the worker role to assign this task to.
func Assignee(role string) NodeOption {
	return func(n *Node) {
		n.WithAssignee(role)
	}
}

// ChainBuilder provides a fluent API for constructing workflow DAGs.
type ChainBuilder struct {
	nodes   []*Node
	current *Node
}

// NewChain creates a new ChainBuilder for constructing a workflow DAG.
func NewChain() *ChainBuilder {
	return &ChainBuilder{
		nodes: []*Node{},
	}
}

// Node adds a new node to the chain and sets it as the current node for subsequent calls.
func (b *ChainBuilder) Node(key, name, template string, opts ...NodeOption) *ChainBuilder {
	node := NewNode(key, name, template)
	for _, opt := range opts {
		opt(node)
	}
	b.nodes = append(b.nodes, node)
	b.current = node
	return b
}

// Build validates the chain and returns an immutable Chain.
// Returns validation errors for: empty chain, duplicate nodes, duplicate outputs,
// dangling inputs, unknown After references, and cycles.
func (b *ChainBuilder) Build() (*Chain, error) {
	// Validate not empty
	if len(b.nodes) == 0 {
		return nil, ErrChainEmpty
	}

	// Build internal maps and validate duplicates
	nodesByKey := make(map[string]*Node)
	producers := make(map[string]*Node)

	for _, node := range b.nodes {
		// Check for duplicate node keys
		if _, exists := nodesByKey[node.Key()]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateNode, node.Key())
		}
		nodesByKey[node.Key()] = node

		// Check for duplicate outputs
		for _, output := range node.Outputs() {
			if existing, exists := producers[output.Filename()]; exists {
				return nil, fmt.Errorf("%w: %s (produced by both %s and %s)",
					ErrDuplicateOutput, output.Filename(), existing.Key(), node.Key())
			}
			producers[output.Filename()] = node
		}
	}

	// Validate all inputs have producers
	for _, node := range b.nodes {
		for _, input := range node.Inputs() {
			if _, exists := producers[input.Filename()]; !exists {
				return nil, fmt.Errorf("%w: %s (required by %s)",
					ErrDanglingInput, input.Filename(), node.Key())
			}
		}
	}

	// Validate all After references exist
	for _, node := range b.nodes {
		for _, afterKey := range node.After() {
			if _, exists := nodesByKey[afterKey]; !exists {
				return nil, fmt.Errorf("%w: %s (referenced by %s)",
					ErrUnknownAfterNode, afterKey, node.Key())
			}
		}
	}

	chain := &Chain{
		nodes:      b.nodes,
		nodesByKey: nodesByKey,
		producers:  producers,
	}

	// Detect cycles
	if err := chain.detectCycles(); err != nil {
		return nil, err
	}

	return chain, nil
}

// Chain represents an immutable workflow DAG after validation.
type Chain struct {
	nodes      []*Node
	nodesByKey map[string]*Node
	producers  map[string]*Node // artifact filename -> producing node
}

// Nodes returns all nodes in the chain.
func (c *Chain) Nodes() []*Node {
	return c.nodes
}

// RootNodes returns nodes with no inputs and no after dependencies.
// These are the starting points of the DAG.
func (c *Chain) RootNodes() []*Node {
	var roots []*Node
	for _, node := range c.nodes {
		if len(node.Inputs()) == 0 && len(node.After()) == 0 {
			roots = append(roots, node)
		}
	}
	return roots
}

// DependenciesOf returns the nodes that the given node depends on.
// Dependencies come from two sources:
// 1. Nodes that produce artifacts listed in the node's inputs
// 2. Nodes explicitly listed in the node's After() list
func (c *Chain) DependenciesOf(key string) []*Node {
	node, ok := c.nodesByKey[key]
	if !ok {
		return nil
	}

	seen := make(map[string]bool)
	var deps []*Node

	// Add dependencies from artifact inputs
	for _, input := range node.Inputs() {
		if producer, ok := c.producers[input.Filename()]; ok {
			if !seen[producer.Key()] {
				seen[producer.Key()] = true
				deps = append(deps, producer)
			}
		}
	}

	// Add explicit After dependencies
	for _, afterKey := range node.After() {
		if afterNode, ok := c.nodesByKey[afterKey]; ok {
			if !seen[afterKey] {
				seen[afterKey] = true
				deps = append(deps, afterNode)
			}
		}
	}

	return deps
}

// detectCycles uses DFS with a recursion stack to detect cycles in the DAG.
// Returns an error if a cycle is detected.
func (c *Chain) detectCycles() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(key string) error
	dfs = func(key string) error {
		visited[key] = true
		recStack[key] = true

		// Get all dependencies of this node
		for _, dep := range c.DependenciesOf(key) {
			depKey := dep.Key()
			if !visited[depKey] {
				if err := dfs(depKey); err != nil {
					return err
				}
			} else if recStack[depKey] {
				return fmt.Errorf("%w: %s -> %s", ErrCycleDetected, key, depKey)
			}
		}

		recStack[key] = false
		return nil
	}

	for _, node := range c.nodes {
		if !visited[node.Key()] {
			if err := dfs(node.Key()); err != nil {
				return err
			}
		}
	}
	return nil
}
