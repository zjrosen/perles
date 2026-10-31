// Package beads implements the application layer for the beads issue tracking system.
//
// This package serves as a facade that bridges the domain layer to infrastructure concerns:
//   - Provides port interfaces for database and CLI operations
//   - Implements infrastructure adapters (Dolt server client, BD CLI executor)
//   - Offers a Service facade for high-level operations
//
// # Architecture
//
// The application layer depends on:
//   - Domain layer (internal/domain/beads): pure domain types and logic
//   - Infrastructure: database/sql for the Dolt SQL server, os/exec for BD CLI
//
// This separation ensures the domain layer remains free of I/O concerns and can be
// tested in isolation.
//
// # Ports (Interfaces)
//
// The package defines several port interfaces:
//   - VersionReader: reads database version
//   - CommentReader: reads issue comments
//   - IssueReader: reads issue details
//   - IssueWriter: mutates issues via CLI
//
// # Infrastructure Adapters
//
// DoltClient implements the read ports (VersionReader, CommentReader).
// BDExecutor implements both IssueReader and IssueWriter via the bd CLI.
//
// # Import Aliasing
//
// Note: This package has the same name as the domain beads package. When importing both,
// use aliasing to disambiguate:
//
//	import (
//	    domainbeads "github.com/zjrosen/perles/internal/beads/domain"
//	    appbeads "github.com/zjrosen/perles/internal/beads/application"
//	)
package application
