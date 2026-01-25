// Package flags provides feature flag support for controlled feature rollout.
// Flags are read-only after initialization and provide safe defaults for unknown flags.
package flags

import (
	"maps"

	"github.com/zjrosen/perles/internal/log"
)

// Flag name constants for type-safe flag access.
const (
	// FlagSessionResume controls whether session resumption (ctrl+r) is enabled.
	FlagSessionResume = "session-resume"

	// FlagRemoveWorktree controls if existing orchestration mode removes the worktree.
	FlagRemoveWorktree = "remove-worktree"

	// FlagSessionPersistence controls whether workflow sessions are persisted to SQLite.
	// When disabled, falls back to in-memory registry (no persistence across restarts).
	FlagSessionPersistence = "session-persistence"
)

// Registry holds feature flag state loaded from configuration.
// Flags are read-only after initialization.
type Registry struct {
	flags map[string]bool
}

// New creates a Registry from a config map.
// If flags is nil, an empty registry is created (all flags disabled).
func New(flags map[string]bool) *Registry {
	if flags == nil {
		flags = make(map[string]bool)
	}
	r := &Registry{flags: flags}
	log.Debug(log.CatConfig, "Feature flags initialized", "count", len(flags), "flags", r.All())
	return r
}

// Enabled returns true if the named flag is enabled.
// Returns false for unknown flags (safe default).
// Returns false when called on nil registry (nil-safe).
func (r *Registry) Enabled(name string) bool {
	if r == nil || r.flags == nil {
		return false
	}
	value, exists := r.flags[name]
	if !exists {
		log.Debug(log.CatConfig, "Unknown flag accessed", "flag", name, "result", false)
		return false
	}
	return value
}

// All returns a copy of all flags (for debugging/logging).
// Returns an empty map if the registry is nil.
func (r *Registry) All() map[string]bool {
	if r == nil || r.flags == nil {
		return make(map[string]bool)
	}
	result := make(map[string]bool, len(r.flags))
	maps.Copy(result, r.flags)
	return result
}
