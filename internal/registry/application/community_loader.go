package registry

import (
	"io/fs"
	"strings"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/registry/domain"
)

// defaultCommunityNamespace is prepended to bare keys (without "/") in EnabledIDs.
// This allows users to write community_workflows: ["joke-contest"] instead of
// the fully-qualified "workflow/joke-contest".
const defaultCommunityNamespace = "workflow"

// CommunitySource holds the filesystem and opt-in filter for community workflows.
// A nil CommunitySource or empty EnabledIDs means no community workflows are loaded.
type CommunitySource struct {
	FS         fs.FS    // Community embedded filesystem
	EnabledIDs []string // Opt-in filter: only these namespace/key IDs are loaded (empty = nothing loaded)
}

// LoadCommunityRegistryFromFS loads community workflow registrations from an embedded filesystem.
// It follows the user loader pattern: nil source or empty EnabledIDs returns nil, nil, nil.
// On any loading error (including zero registrations), it logs a WARN and returns nil, FS, nil
// (never crashes startup). Only registrations matching EnabledIDs are returned.
// Unmatched EnabledIDs produce WARN logs.
//
// Templates are resolved in source.FS first. When builtinFS is non-nil, templates not found
// there fall back to builtinFS (the embedded built-in templates). The returned FS reads
// templates with the same precedence and should be used to render the returned registrations.
func LoadCommunityRegistryFromFS(source *CommunitySource, builtinFS fs.FS) ([]*registry.Registration, fs.FS, error) {
	// Opt-in gate: nil source or empty EnabledIDs means nothing to load
	if source == nil || len(source.EnabledIDs) == 0 {
		return nil, nil, nil
	}

	templateFS := newLayeredFS(source.FS, builtinFS)

	// Load all community registrations from the filesystem
	regs, err := loadRegistryFromYAML(source.FS, registry.SourceCommunity, loadOptions{templateFS: templateFS})
	if err != nil {
		log.Warn(log.CatConfig, "loading community registrations", "error", err.Error())
		return nil, templateFS, nil
	}

	// Build a lookup of available registrations by namespace/key
	available := make(map[string]*registry.Registration, len(regs))
	for _, r := range regs {
		id := r.Namespace() + "/" + r.Key()
		available[id] = r
	}

	// Filter registrations against EnabledIDs.
	// Bare keys (without "/") are normalized to "workflow/<key>" for convenience,
	// so users can write community_workflows: ["joke-contest"] instead of "workflow/joke-contest".
	var filtered []*registry.Registration
	for _, id := range source.EnabledIDs {
		normalizedID := normalizeCommunityID(id)
		r, ok := available[normalizedID]
		if !ok {
			log.Warn(log.CatConfig, "enabled community workflow not found",
				"id", id,
				"available", availableIDs(regs))
			continue
		}
		filtered = append(filtered, r)
	}

	return filtered, templateFS, nil
}

// normalizeCommunityID ensures a community workflow ID is in namespace/key format.
// Bare keys (e.g., "joke-contest") are prefixed with the default namespace ("workflow/joke-contest").
// Fully-qualified keys (e.g., "workflow/joke-contest") are returned as-is.
func normalizeCommunityID(id string) string {
	if strings.Contains(id, "/") {
		return id
	}
	return defaultCommunityNamespace + "/" + id
}

// availableIDs returns a comma-separated list of namespace/key IDs for logging.
func availableIDs(regs []*registry.Registration) string {
	if len(regs) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for i, r := range regs {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(r.Namespace())
		b.WriteByte('/')
		b.WriteString(r.Key())
	}
	return b.String()
}
