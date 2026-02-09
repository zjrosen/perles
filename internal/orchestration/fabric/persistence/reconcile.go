package persistence

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	"github.com/zjrosen/perles/internal/orchestration/fabric/repository"
)

// ReconciliationState classifies the startup state of JSONL and SQLite.
type ReconciliationState string

const (
	// StateJSONLOnly means JSONL has content but SQLite has no rows. Backfill needed.
	StateJSONLOnly ReconciliationState = "jsonl_only"
	// StateSQLiteOnly means SQLite has rows but no JSONL file exists. Serve with warning.
	StateSQLiteOnly ReconciliationState = "sqlite_only"
	// StateBothMatch means both JSONL and SQLite exist with matching counts.
	StateBothMatch ReconciliationState = "both_match"
	// StateBothMismatch means both exist but counts differ. Serve from SQLite with diagnostics.
	StateBothMismatch ReconciliationState = "both_mismatch"
)

// migrationCompleteMarkerFile is the filename used to indicate that a JSONL→SQLite
// backfill has completed successfully. Its presence prevents repeated replay.
const migrationCompleteMarkerFile = ".fabric_migration_complete"

// ReconciliationResult holds the outcome of state detection and reconciliation.
type ReconciliationResult struct {
	State ReconciliationState

	ThreadCountJSONL  int
	ThreadCountSQLite int
	DepCountJSONL     int
	DepCountSQLite    int

	OrphanEdges       int
	UnresolvedParents int
	BackfillDuration  time.Duration
	Diagnostics       []string
}

// ReconciliationConfig holds thresholds for deciding whether to fall back
// to JSONL-only mode.
type ReconciliationConfig struct {
	// MaxOrphanEdges is the maximum number of dependency edges referencing
	// non-existent threads before triggering fallback. Default: 0.
	MaxOrphanEdges int

	// MaxUnresolvedParentRate is the maximum fraction of threads whose parent
	// dependency cannot be resolved before triggering fallback. Default: 0.005.
	MaxUnresolvedParentRate float64

	// MaxBackfillDuration is the maximum wall-clock time allowed for backfill
	// before triggering fallback. Default: 30s.
	MaxBackfillDuration time.Duration
}

// DefaultReconciliationConfig returns the default threshold configuration.
func DefaultReconciliationConfig() ReconciliationConfig {
	return ReconciliationConfig{
		MaxOrphanEdges:          0,
		MaxUnresolvedParentRate: 0.005,
		MaxBackfillDuration:     30 * time.Second,
	}
}

// DetectState examines JSONL and SQLite to determine which data sources are populated.
// It classifies the startup into one of four states:
//   - jsonl_only:     JSONL has events, SQLite is empty → backfill needed
//   - sqlite_only:    SQLite has rows, JSONL missing/empty → serve with warning
//   - both_match:     Both present with equal thread+dep counts → normal operation
//   - both_mismatch:  Both present but counts differ → serve SQLite + diagnostics
func DetectState(
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
	jsonlPath string,
) (*ReconciliationResult, error) {
	result := &ReconciliationResult{}

	// Check JSONL — load and count thread/dep content (not just file existence)
	if hasJSONLContent(jsonlPath) {
		events, err := LoadPersistedEvents(filepath.Dir(jsonlPath))
		if err != nil {
			return nil, fmt.Errorf("loading JSONL events for reconciliation: %w", err)
		}
		result.ThreadCountJSONL, result.DepCountJSONL = countThreadsAndDeps(events)
	}
	jsonlHasGraphData := result.ThreadCountJSONL > 0 || result.DepCountJSONL > 0

	// Check SQLite
	sqliteThreadCount, sqliteDepCount, err := countSQLiteRows(threads, deps)
	if err != nil {
		return nil, fmt.Errorf("counting SQLite rows for reconciliation: %w", err)
	}
	result.ThreadCountSQLite = sqliteThreadCount
	result.DepCountSQLite = sqliteDepCount

	sqliteHasRows := sqliteThreadCount > 0 || sqliteDepCount > 0

	// Classify state based on whether each source has graph data (threads/deps)
	switch {
	case jsonlHasGraphData && !sqliteHasRows:
		result.State = StateJSONLOnly
		result.Diagnostics = append(result.Diagnostics,
			fmt.Sprintf("JSONL has %d threads and %d deps; SQLite is empty — backfill required",
				result.ThreadCountJSONL, result.DepCountJSONL))

	case !jsonlHasGraphData && sqliteHasRows:
		result.State = StateSQLiteOnly
		result.Diagnostics = append(result.Diagnostics,
			fmt.Sprintf("SQLite has %d threads and %d deps; JSONL missing/empty — serving from SQLite with warning",
				sqliteThreadCount, sqliteDepCount))

	case jsonlHasGraphData && sqliteHasRows:
		if result.ThreadCountJSONL == sqliteThreadCount && result.DepCountJSONL == sqliteDepCount {
			result.State = StateBothMatch
			result.Diagnostics = append(result.Diagnostics,
				fmt.Sprintf("JSONL and SQLite match: %d threads, %d deps",
					sqliteThreadCount, sqliteDepCount))
		} else {
			result.State = StateBothMismatch
			result.Diagnostics = append(result.Diagnostics,
				fmt.Sprintf("JSONL/SQLite mismatch: JSONL has %d threads/%d deps, SQLite has %d threads/%d deps",
					result.ThreadCountJSONL, result.DepCountJSONL,
					sqliteThreadCount, sqliteDepCount))
		}

	default:
		// Neither has content — treat as both_match (clean slate)
		result.State = StateBothMatch
		result.Diagnostics = append(result.Diagnostics, "Both JSONL and SQLite are empty — clean slate")
	}

	log.Info(log.CatOrch, "Reconciliation state detected",
		"state", string(result.State),
		"jsonl_threads", result.ThreadCountJSONL,
		"jsonl_deps", result.DepCountJSONL,
		"sqlite_threads", result.ThreadCountSQLite,
		"sqlite_deps", result.DepCountSQLite,
	)

	return result, nil
}

// Backfill replays JSONL events into SQLite repositories.
// Phase 1: insert threads (channels, messages, artifacts).
// Phase 2: insert dependencies (child_of, reply_to, references).
// Phase 3: validate — count orphan edges and unresolved parents.
//
// Backfill is idempotent: a migration-complete marker file is written on success
// and checked at the start. If the marker already exists, Backfill is a no-op.
func Backfill(
	result *ReconciliationResult,
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
	jsonlPath string,
) error {
	// Check for migration-complete marker — skip if already done.
	markerPath := migrationMarkerPath(jsonlPath)
	if markerExists(markerPath) {
		result.Diagnostics = append(result.Diagnostics, "Migration-complete marker present — skipping backfill")
		log.Info(log.CatOrch, "Backfill skipped: migration-complete marker already present",
			"marker_path", markerPath)
		return nil
	}

	start := time.Now()

	events, err := LoadPersistedEvents(filepath.Dir(jsonlPath))
	if err != nil {
		return fmt.Errorf("loading JSONL events for backfill: %w", err)
	}

	// Phase 1: insert threads
	threadsInserted := 0
	for _, pe := range events {
		if pe.Event.Thread == nil {
			continue
		}
		thread := *pe.Event.Thread
		if _, err := threads.Create(thread); err != nil {
			// Skip duplicates — Create should handle idempotently
			continue
		}
		threadsInserted++
	}

	// Phase 2: insert dependencies
	depsInserted := 0
	for _, pe := range events {
		depEdges := extractDeps(pe)
		for _, dep := range depEdges {
			if err := deps.Add(dep); err != nil {
				continue
			}
			depsInserted++
		}
	}

	result.BackfillDuration = time.Since(start)

	// Phase 3: validate
	orphans, unresolved, err := validateBackfill(threads, deps)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics,
			fmt.Sprintf("Backfill validation error: %v", err))
		return fmt.Errorf("validating backfill: %w", err)
	}
	result.OrphanEdges = orphans
	result.UnresolvedParents = unresolved

	// Update SQLite counts in result
	sqliteThreads, sqliteDeps, err := countSQLiteRows(threads, deps)
	if err != nil {
		return fmt.Errorf("counting SQLite rows after backfill: %w", err)
	}
	result.ThreadCountSQLite = sqliteThreads
	result.DepCountSQLite = sqliteDeps

	result.Diagnostics = append(result.Diagnostics,
		fmt.Sprintf("Backfill complete: %d threads, %d deps inserted in %v (orphan_edges=%d, unresolved_parents=%d)",
			threadsInserted, depsInserted, result.BackfillDuration, orphans, unresolved))

	// Write migration-complete marker
	if err := writeMigrationMarker(markerPath); err != nil {
		return fmt.Errorf("writing migration-complete marker: %w", err)
	}

	result.Diagnostics = append(result.Diagnostics, "Migration-complete marker written")

	log.Info(log.CatOrch, "Backfill completed",
		"threads_inserted", threadsInserted,
		"deps_inserted", depsInserted,
		"duration", result.BackfillDuration,
		"orphan_edges", orphans,
		"unresolved_parents", unresolved,
	)

	return nil
}

// Reconcile repairs a mismatch state by filling missing rows idempotently.
// It replays JSONL events into SQLite, relying on idempotent Create/Add to skip
// existing records. Returns an updated result with diagnostics.
func Reconcile(
	result *ReconciliationResult,
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
	jsonlPath string,
) (*ReconciliationResult, error) {
	if result.State != StateBothMismatch {
		result.Diagnostics = append(result.Diagnostics,
			fmt.Sprintf("Reconcile called in non-mismatch state %q — no action taken", result.State))
		return result, nil
	}

	events, err := LoadPersistedEvents(filepath.Dir(jsonlPath))
	if err != nil {
		return result, fmt.Errorf("loading JSONL events for reconciliation: %w", err)
	}

	// Idempotent fill: replay threads
	threadsFilled := 0
	for _, pe := range events {
		if pe.Event.Thread == nil {
			continue
		}
		thread := *pe.Event.Thread
		// Create is idempotent — existing threads are skipped
		if _, err := threads.Create(thread); err != nil {
			continue
		}
		threadsFilled++
	}

	// Idempotent fill: replay dependencies
	depsFilled := 0
	for _, pe := range events {
		depEdges := extractDeps(pe)
		for _, dep := range depEdges {
			if err := deps.Add(dep); err != nil {
				continue
			}
			depsFilled++
		}
	}

	// Recount SQLite
	sqliteThreads, sqliteDeps, err := countSQLiteRows(threads, deps)
	if err != nil {
		return result, fmt.Errorf("counting SQLite rows after reconciliation: %w", err)
	}
	result.ThreadCountSQLite = sqliteThreads
	result.DepCountSQLite = sqliteDeps

	// Validate
	orphans, unresolved, err := validateBackfill(threads, deps)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics,
			fmt.Sprintf("Reconciliation validation error: %v", err))
	} else {
		result.OrphanEdges = orphans
		result.UnresolvedParents = unresolved
	}

	result.Diagnostics = append(result.Diagnostics,
		fmt.Sprintf("Reconciliation complete: filled %d threads, %d deps (SQLite now: %d threads, %d deps, orphan_edges=%d, unresolved_parents=%d)",
			threadsFilled, depsFilled, sqliteThreads, sqliteDeps, result.OrphanEdges, result.UnresolvedParents))

	log.Info(log.CatOrch, "Reconciliation completed",
		"threads_filled", threadsFilled,
		"deps_filled", depsFilled,
		"sqlite_threads", sqliteThreads,
		"sqlite_deps", sqliteDeps,
		"orphan_edges", result.OrphanEdges,
		"unresolved_parents", result.UnresolvedParents,
	)

	return result, nil
}

// ShouldFallback returns true if any reconciliation threshold is exceeded,
// indicating that the SQLite read path should be disabled for this session.
func ShouldFallback(result *ReconciliationResult, config ReconciliationConfig) bool {
	if result.OrphanEdges > config.MaxOrphanEdges {
		return true
	}

	if result.BackfillDuration > config.MaxBackfillDuration {
		return true
	}

	// Calculate unresolved parent rate against total thread count.
	totalThreads := result.ThreadCountSQLite
	if totalThreads == 0 {
		totalThreads = result.ThreadCountJSONL
	}
	if totalThreads > 0 {
		rate := float64(result.UnresolvedParents) / float64(totalThreads)
		if rate > config.MaxUnresolvedParentRate {
			return true
		}
	}

	return false
}

// --- helpers ---

// hasJSONLContent returns true if the JSONL file exists and has non-zero size.
func hasJSONLContent(jsonlPath string) bool {
	info, err := os.Stat(jsonlPath)
	if err != nil {
		return false
	}
	return info.Size() > 0
}

// countThreadsAndDeps counts distinct threads and dependency edges in JSONL events.
func countThreadsAndDeps(events []PersistedEvent) (threads, deps int) {
	threadIDs := make(map[string]struct{})
	depKeys := make(map[string]struct{})

	for _, pe := range events {
		if pe.Event.Thread != nil {
			threadIDs[pe.Event.Thread.ID] = struct{}{}
		}
		for _, dep := range extractDeps(pe) {
			depKeys[dep.Key()] = struct{}{}
		}
	}
	return len(threadIDs), len(depKeys)
}

// countSQLiteRows counts threads and dependencies in the SQLite repositories.
func countSQLiteRows(
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
) (threadCount, depCount int, err error) {
	// List all threads (no filter, no limit)
	allThreads, err := threads.List(repository.ListOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("listing threads: %w", err)
	}
	threadCount = len(allThreads)

	// Count deps by summing all root children (or listing all threads' deps).
	// We iterate over all threads and count unique dep edges.
	depSet := make(map[string]struct{})
	for _, t := range allThreads {
		children, err := deps.GetChildren(t.ID, nil)
		if err != nil {
			return 0, 0, fmt.Errorf("getting children for thread %s: %w", t.ID, err)
		}
		for _, d := range children {
			depSet[d.Key()] = struct{}{}
		}
	}
	depCount = len(depSet)

	return threadCount, depCount, nil
}

// extractDeps extracts dependency edges from a persisted event.
func extractDeps(pe PersistedEvent) []domain.Dependency {
	event := pe.Event
	var result []domain.Dependency

	switch event.Type {
	case fabric.EventMessagePosted:
		if event.Thread != nil && event.ChannelID != "" {
			result = append(result,
				domain.NewDependency(event.Thread.ID, event.ChannelID, domain.RelationChildOf))
		}

	case fabric.EventReplyPosted:
		if event.Thread != nil && event.ParentID != "" {
			result = append(result,
				domain.NewDependency(event.Thread.ID, event.ParentID, domain.RelationReplyTo))
		}

	case fabric.EventArtifactAdded:
		if event.Thread != nil && event.ChannelID != "" {
			result = append(result,
				domain.NewDependency(event.Thread.ID, event.ChannelID, domain.RelationReferences))
		}
	}

	return result
}

// validateBackfill checks for orphan edges and unresolved parents.
// An orphan edge is a dependency referencing a thread ID that doesn't exist.
// An unresolved parent is a thread that has a child_of dep pointing to a missing thread.
func validateBackfill(
	threads repository.ThreadRepository,
	deps repository.DependencyRepository,
) (orphanEdges, unresolvedParents int, err error) {
	allThreads, err := threads.List(repository.ListOptions{})
	if err != nil {
		return 0, 0, fmt.Errorf("listing threads for validation: %w", err)
	}

	threadExists := make(map[string]struct{}, len(allThreads))
	for _, t := range allThreads {
		threadExists[t.ID] = struct{}{}
	}

	for _, t := range allThreads {
		parents, err := deps.GetParents(t.ID, nil)
		if err != nil {
			return 0, 0, fmt.Errorf("getting parents for thread %s: %w", t.ID, err)
		}
		for _, p := range parents {
			if _, ok := threadExists[p.DependsOnID]; !ok {
				orphanEdges++
				if p.Relation == domain.RelationChildOf {
					unresolvedParents++
				}
			}
		}
	}

	return orphanEdges, unresolvedParents, nil
}

// migrationMarkerPath returns the path for the migration-complete marker file
// relative to the JSONL file's directory.
func migrationMarkerPath(jsonlPath string) string {
	return filepath.Join(filepath.Dir(jsonlPath), migrationCompleteMarkerFile)
}

// markerExists checks if the migration-complete marker file exists.
func markerExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writeMigrationMarker creates the migration-complete marker file.
func writeMigrationMarker(path string) error {
	content := fmt.Sprintf("migration completed at %s\n", time.Now().Format(time.RFC3339))
	return os.WriteFile(path, []byte(content), 0644) //nolint:gosec // internal marker file
}
