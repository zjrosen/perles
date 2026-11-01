package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/log"
)

// This file holds the bd reads that build an embedded-workspace snapshot
// (the export and the custom statuses) and the bd version gate for embedded
// workspaces. The reads are functions over a BDRunner, so the snapshot
// backend composes them with the blocked-flag reads; none of them keeps
// state between calls.

// snapshotSourceTimeout bounds each bd read of a snapshot build. bd waits up
// to 30s on a workspace lock, and a failed build keeps the previous snapshot
// and is retried, so a read is not given defaultBDTimeout's migration room.
const snapshotSourceTimeout = 30 * time.Second

// exportOwnerSentinel is the value perles gives bd's export.exclude_owners
// and export.exclude_owner keys for its own export. It matches no created_by,
// so bd excludes no issue. A blank value is not an option: bd adds the
// singular key's value to its exclude set as "", which drops every issue
// whose created_by is empty.
const exportOwnerSentinel = "__perles_none__"

// snapshotExportArgs is perles' export: every issues-table record (infra
// types, templates and gates included) plus memories, which DecodeExport
// skips.
var snapshotExportArgs = []string{"--readonly", "export", "--all"}

// snapshotExportEnv overrides export.exclude_owners and export.exclude_owner
// for perles' export subprocess only. bd reads the BD_EXPORT_* environment
// ahead of its config.yaml, so the user's config and their own exports are
// untouched. A value written straight into the database config table (no bd
// command does that) is not overridden.
var snapshotExportEnv = []string{
	"BD_EXPORT_EXCLUDE_OWNERS=" + exportOwnerSentinel,
	"BD_EXPORT_EXCLUDE_OWNER=" + exportOwnerSentinel,
}

// snapshotStatusesArgs lists bd's built-in and custom statuses. --readonly
// matters here: statuses is not on bd's read-only command list, so without
// it bd would open a writable store that may migrate.
var snapshotStatusesArgs = []string{"--readonly", "statuses", "--json"}

// bdBuiltinStatuses are bd's built-in statuses (cmd/bd/statuses.go). An
// issue with any other status makes ReadCustomStatuses ask bd for the
// workspace's custom statuses.
var bdBuiltinStatuses = []string{"open", "in_progress", "blocked", "deferred", "closed", "pinned", "hooked"}

// ReadSnapshotExport runs `bd --readonly export --all` with export.exclude_owners
// neutralized (snapshotExportEnv) and returns bd's stdout, the JSONL that
// DecodeExport reads. The read is bounded by snapshotSourceTimeout and ends
// with ctx. A failure is run's error: for NewBDRunner, the translated *bdError
// with its hint (a pending migration, bd not on PATH, a timeout).
func ReadSnapshotExport(ctx context.Context, run BDRunner) (string, error) {
	return run(ctx, BDRequest{
		Args:    slices.Clone(snapshotExportArgs),
		Env:     slices.Clone(snapshotExportEnv),
		Timeout: snapshotSourceTimeout,
	})
}

// ReadCustomStatuses returns the custom statuses a snapshot of issues needs,
// so that issues in a custom "active" status count as ready.
//
// It runs `bd --readonly statuses --json` only when some issue has a status
// outside bdBuiltinStatuses; otherwise it returns nil without running bd.
// The read is bounded by snapshotSourceTimeout and ends with ctx, and a bd
// failure is run's error unchanged.
//
// bd ignores a failure to look up its custom statuses and reports none, so
// an empty result while issues use statuses that bd's own built_in_statuses
// do not list is logged as a warning: those issues then never count as ready.
func ReadCustomStatuses(ctx context.Context, run BDRunner, issues []ExportIssue) ([]CustomStatus, error) {
	if len(statusesOutside(issues, bdBuiltinStatuses)) == 0 {
		return nil, nil
	}
	out, err := run(ctx, BDRequest{Args: slices.Clone(snapshotStatusesArgs), Timeout: snapshotSourceTimeout})
	if err != nil {
		return nil, err
	}
	statuses, err := parseBDStatuses(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd statuses output: %w", err)
	}

	builtins := bdBuiltinStatuses
	if len(statuses.BuiltIn) > 0 {
		builtins = statuses.BuiltIn
	}
	if len(statuses.Custom) == 0 {
		if unknown := statusesOutside(issues, builtins); len(unknown) > 0 {
			log.Warn(log.CatBeads, "bd statuses reported no custom statuses, but issues use statuses outside bd's built-ins; they will not count as ready",
				"statuses", strings.Join(unknown, ","))
		}
	}
	return statuses.Custom, nil
}

// bdStatuses is the parsed output of `bd statuses --json`.
type bdStatuses struct {
	// BuiltIn are the names in built_in_statuses.
	BuiltIn []string
	// Custom are custom_statuses with a name, in bd's order.
	Custom []CustomStatus
}

// parseBDStatuses parses `bd statuses --json`:
// {"built_in_statuses":[{name, category, ...}], "custom_statuses":[{name, category}]}.
// bd omits custom_statuses when there are none.
func parseBDStatuses(out string) (bdStatuses, error) {
	payload, err := bdJSONPayload(out)
	if err != nil {
		return bdStatuses{}, err
	}
	type statusJSON struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	var raw struct {
		BuiltInStatuses []statusJSON `json:"built_in_statuses"`
		CustomStatuses  []statusJSON `json:"custom_statuses"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return bdStatuses{}, err
	}

	var statuses bdStatuses
	for _, s := range raw.BuiltInStatuses {
		if s.Name != "" {
			statuses.BuiltIn = append(statuses.BuiltIn, s.Name)
		}
	}
	for _, s := range raw.CustomStatuses {
		if s.Name != "" {
			statuses.Custom = append(statuses.Custom, CustomStatus(s))
		}
	}
	return statuses, nil
}

// statusesOutside returns the distinct statuses of issues that are not in
// builtins, sorted.
func statusesOutside(issues []ExportIssue, builtins []string) []string {
	var outside []string
	for i := range issues {
		status := issues[i].Status
		if !slices.Contains(builtins, status) && !slices.Contains(outside, status) {
			outside = append(outside, status)
		}
	}
	slices.Sort(outside)
	return outside
}

// CheckEmbeddedBDVersion checks that the bd perles reads an embedded
// workspace through is at least domain.MinEmbeddedBeadsVersion. version and
// contextErr are the workspace's EmbeddedWorkspaceError.BDVersion and
// BDContextErr: the raw bd_version `bd context` printed and the error that
// stopped perles from using its answer. Versions compare without their
// pre-release or build annotation ("1.3.2 (6ebaacba8: main@6ebaacba8)" is
// 1.3.2).
//
// The result, by case:
//   - version >= 1.0.5: nil, whatever contextErr says.
//   - an older version, including bd before 1.0.0: an
//     *UnsupportedWorkspaceError of kind WorkspaceEmbeddedDolt with the Detail
//     "embedded workspaces need bd >= 1.0.5 (found X)".
//   - no version, and contextErr wraps exec.ErrNotFound (bd is not on PATH):
//     nil. The export then fails with "bd CLI not found on PATH", which names
//     the cause better than a version could.
//   - no version otherwise (bd context failed, printed an empty bd_version,
//     or contextErr is nil): the same error kind with the Detail
//     "embedded workspaces need bd >= 1.0.5 (could not read the bd version:
//     <reason>)", where reason is contextErr's text, or "bd context reported
//     no bd_version".
func CheckEmbeddedBDVersion(beadsDir, version string, contextErr error) error {
	if v := baseVersion(version); v != "" {
		if domain.CompareVersions(v, domain.MinEmbeddedBeadsVersion) >= 0 {
			return nil
		}
		return embeddedBDVersionError(beadsDir, "found "+v)
	}
	if errors.Is(contextErr, exec.ErrNotFound) {
		return nil
	}
	return embeddedBDVersionError(beadsDir, "could not read the bd version: "+unreadBDVersionReason(contextErr))
}

// embeddedBDVersionError refuses the embedded workspace at beadsDir for its
// bd version; cause says what the version was or why it is unknown.
func embeddedBDVersionError(beadsDir, cause string) *UnsupportedWorkspaceError {
	return &UnsupportedWorkspaceError{
		Kind:     WorkspaceEmbeddedDolt,
		BeadsDir: beadsDir,
		Detail:   fmt.Sprintf("embedded workspaces need bd >= %s (%s)", domain.MinEmbeddedBeadsVersion, cause),
	}
}

// unreadBDVersionReason says why bd context yielded no version, as one line.
// A nil contextErr, or bd context's own rejection of an empty bd_version,
// means bd answered without one.
func unreadBDVersionReason(contextErr error) string {
	tooOld, isTooOld := errors.AsType[*bdContextTooOldError](contextErr)
	if contextErr == nil || (isTooOld && strings.TrimSpace(tooOld.Version) == "") {
		return "bd context reported no bd_version"
	}
	return truncateRunes(collapseWhitespace(contextErr.Error()), maxBDMessageRunes)
}
