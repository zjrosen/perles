// Package registry implements the application layer for the registration registry system.
//
// This package serves as a facade that bridges the domain layer to infrastructure concerns:
//   - Loads registrations from YAML configuration files
//   - Reads and renders templates from embedded filesystem
//   - Provides high-level operations for template retrieval and rendering
//
// # Architecture
//
// The application layer depends on:
//   - Domain layer (internal/domain/registry): pure domain types and logic
//   - Infrastructure: embed.FS for file access, YAML parsing for configuration
//
// This separation ensures the domain layer remains free of I/O concerns and can be
// tested in isolation.
//
// # RegistryService
//
// RegistryService is the main entry point. It provides:
//   - List, GetByType, GetByKey, GetByLabels: delegate to domain Registry
//   - GetTemplate: retrieves raw template content by identifier
//   - RenderTemplate: renders a template with context variables (slug, inputs, outputs)
//
// # Template Identifiers
//
// Templates are addressed using qualified identifiers:
//
//	{type}::{key}::{version}::{chain-key}
//
// Example: spec-workflow::planning-standard::v1::research
//
// # YAML Configuration
//
// Registrations are loaded from registry.yaml embedded in the templates filesystem.
// The YAMLLoader handles parsing and converts YAML structures to domain Registration objects.
//
// # Import Aliasing
//
// Note: This package has the same name as the domain registry package. When importing both,
// use aliasing to disambiguate:
//
//	import (
//	    domainreg "github.com/zjrosen/perles/internal/domain/registry"
//	    appreg "github.com/zjrosen/perles/internal/application/registry"
//	)
//
// Or reference the application package through RegistryService without a separate alias.
package registry
