// Package registry implements the domain layer for the registration registry system.
//
// This package follows Domain-Driven Design (DDD) principles:
//   - Contains only pure Go code with standard library imports (no external dependencies)
//   - Defines entity types (Registration, Chain, Node) and value objects (Artifact, Identifier)
//   - Implements domain logic (DAG topological sort, cycle detection, label matching)
//   - Has no knowledge of infrastructure concerns (file I/O, YAML parsing, databases)
//
// # Core Types
//
// Registration represents a registered item with type, key, version, and optional metadata
// including labels and a DAG-based workflow chain. Use RegistrationBuilder for construction.
//
// Chain represents a directed acyclic graph of workflow nodes. It provides:
//   - TopologicalSort: deterministic ordering respecting dependencies
//   - CycleDetect: identifies circular dependencies
//   - Validate: ensures the chain forms a valid DAG
//
// Node represents a single step in a workflow chain with inputs, outputs, and dependencies.
//
// Artifact wraps a filename for type-safe artifact references in node inputs/outputs.
//
// Identifier supports parsing of qualified identifiers in format: type::key::version[::chain-key]
//
// # Registry Collection
//
// Registry is the collection type that holds registrations. It provides:
//   - Add/List for managing registrations
//   - GetByType/GetByKey for lookup
//   - GetByLabels for label-based filtering (AND logic)
//   - Labels for discovering all available labels
//
// RegistryProvider is the interface that Registry implements, enabling dependency injection
// and mock substitution in tests.
//
// # Import Aliasing
//
// Note: There is also a workflow.Registry in internal/orchestration/workflow/ for managing
// workflow templates. When importing both packages, use aliasing to disambiguate:
//
//	import (
//	    domainreg "github.com/zjrosen/perles/internal/domain/registry"
//	    "github.com/zjrosen/perles/internal/orchestration/workflow"
//	)
package registry
