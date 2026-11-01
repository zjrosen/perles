package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/zjrosen/perles/internal/log"
)

// This file reads bd's stored is_blocked flags for an embedded-workspace
// snapshot. perles never computes the flag: it takes bd's answer, so
// `blocked = true` and `ready = true` follow bd's rules (custom done statuses,
// blockedness flowing down parent-child, waits-for gates) exactly as server
// mode, which reads the column directly, does.
//
// Two bd reads can answer, both --readonly and bounded by
// snapshotSourceTimeout:
//
//   - The listing, `bd --readonly blocked --json`, on every released bd
//     (>= 1.0.5). It lists the issues whose stored flag is set and for which
//     bd can name a blocker. A set flag with no nameable blocker is a stale
//     flag, the listing's only divergence from the column. Repair stale
//     flags with bd recompute-blocked / bd doctor --fix where available.
//   - The upgrade, `bd --readonly blocked --flags --raw --json`, which answers
//     the stored column itself for the IDs on stdin. It exists only on
//     main-branch bd builds (no release up to v1.3.2 has it), so support is
//     detected once per BlockedFlagSource.

// blockedListingArgs is the listing: the blocked rows with their blockers.
var blockedListingArgs = []string{"--readonly", "blocked", "--json"}

// blockedFlagsArgs asks for the stored column for the IDs on stdin. --flags
// comes before --raw because cobra reports only the first unknown flag, so a
// bd without either says "unknown flag: --flags".
var blockedFlagsArgs = []string{"--readonly", "blocked", "--flags", "--raw", "--json"}

// blockingDepTypes are the dependency types through which bd sets an issue's
// own flag (beads issueops/blocked_consistency.go): blocks and
// conditional-blocks to an open target, parent-child to a blocked parent, and
// a blocking waits-for gate.
var blockingDepTypes = []string{"blocks", "conditional-blocks", "waits-for", "parent-child"}

// externalRefPrefix starts a cross-project dependency target.
const externalRefPrefix = "external:"

// blockedFlagSupport is whether the installed bd has `blocked --flags --raw`.
type blockedFlagSupport int32

const (
	blockedFlagsUnknown blockedFlagSupport = iota
	blockedFlagsSupported
	blockedFlagsUnsupported
)

// BlockedFlagSource reads the issues whose stored is_blocked flag bd reports
// set, for one snapshot backend. It remembers for its lifetime whether the
// installed bd has `blocked --flags --raw`, so a released bd is sent the
// failing probe at most once. It is safe for concurrent use. Create it with
// NewBlockedFlagSource; the zero value is not usable.
type BlockedFlagSource struct {
	run     BDRunner
	support atomic.Int32 // a blockedFlagSupport
	// probing holds a token while a call with unknown support asks bd for
	// --flags, so concurrent first builds probe one at a time and the later
	// ones see the result.
	probing chan struct{}
}

// NewBlockedFlagSource returns a BlockedFlagSource that runs bd through run.
// It has not yet asked bd whether --flags is supported.
func NewBlockedFlagSource(run BDRunner) *BlockedFlagSource {
	return &BlockedFlagSource{run: run, probing: make(chan struct{}, 1)}
}

// ReadBlockedIDs returns the IDs of issues whose stored is_blocked flag bd
// reports set, for a snapshot of issues (the decoded export).
//
// Only candidates can carry the flag: issues that are not closed or pinned
// and have at least one outgoing blocks, conditional-blocks, waits-for or
// parent-child edge. Without candidates bd is not run and the set is empty.
// Otherwise, while --flags is supported or untested, the candidate IDs go to
// `bd --readonly blocked --flags --raw --json` on stdin. When bd rejects
// exactly --flags or --raw as an unknown flag, that is remembered and this
// and every later call use the listing, minus rows whose blockers are all
// external: refs. Any other bd failure is run's error unchanged (for
// NewBDRunner, the translated *bdError).
func (s *BlockedFlagSource) ReadBlockedIDs(ctx context.Context, issues []ExportIssue) (map[string]bool, error) {
	candidates := blockedFlagCandidates(issues)
	if len(candidates) == 0 {
		// Every row the listing or --flags can report set is a candidate.
		return map[string]bool{}, nil
	}
	if s.supportState() != blockedFlagsUnsupported {
		ids, supported, err := s.readFlags(ctx, candidates)
		if err != nil || supported {
			return ids, err
		}
	}
	return readBlockedListing(ctx, s.run)
}

func (s *BlockedFlagSource) supportState() blockedFlagSupport {
	return blockedFlagSupport(s.support.Load())
}

// readFlags runs `bd blocked --flags --raw` on candidates. supported is false,
// with no error, when bd rejected --flags or --raw (now remembered) or another
// call found that out while this one waited to probe.
func (s *BlockedFlagSource) readFlags(ctx context.Context, candidates []string) (ids map[string]bool, supported bool, err error) {
	if s.supportState() == blockedFlagsUnknown {
		select {
		case s.probing <- struct{}{}:
			defer func() { <-s.probing }()
		case <-ctx.Done():
			return nil, false, context.Cause(ctx)
		}
		if s.supportState() == blockedFlagsUnsupported {
			return nil, false, nil
		}
	}

	out, err := s.run(ctx, BDRequest{
		Args:    slices.Clone(blockedFlagsArgs),
		Stdin:   strings.Join(candidates, "\n") + "\n",
		Timeout: snapshotSourceTimeout,
	})
	if err != nil {
		if flag := unknownFlagName(err); flag == "--flags" || flag == "--raw" {
			s.support.Store(int32(blockedFlagsUnsupported))
			log.Info(log.CatBeads, "bd has no blocked --flags --raw; reading blocked flags from the bd blocked listing",
				"unknownFlag", flag)
			return nil, false, nil
		}
		return nil, false, err
	}
	ids, err = parseBlockedFlags(out)
	if err != nil {
		return nil, false, fmt.Errorf("parsing bd blocked --flags output: %w", err)
	}
	s.support.CompareAndSwap(int32(blockedFlagsUnknown), int32(blockedFlagsSupported))
	return ids, true, nil
}

// readBlockedListing runs the listing and returns its IDs minus the rows
// whose blockers are all external: refs.
func readBlockedListing(ctx context.Context, run BDRunner) (map[string]bool, error) {
	out, err := run(ctx, BDRequest{Args: slices.Clone(blockedListingArgs), Timeout: snapshotSourceTimeout})
	if err != nil {
		return nil, err
	}
	ids, err := parseBlockedListing(out)
	if err != nil {
		return nil, fmt.Errorf("parsing bd blocked output: %w", err)
	}
	return ids, nil
}

// blockedFlagCandidates returns, in export order, the IDs of the issues that
// are not closed or pinned and have an outgoing dependency of a
// blockingDepTypes type. bd sets the flag only through such edges, and
// isBlockedFlagExpr ignores it on closed and pinned issues.
func blockedFlagCandidates(issues []ExportIssue) []string {
	var candidates []string
	for i := range issues {
		issue := &issues[i]
		if issue.Status == "closed" || issue.Status == "pinned" {
			continue
		}
		if slices.ContainsFunc(issue.Dependencies, func(d ExportDependency) bool {
			return slices.Contains(blockingDepTypes, d.Type)
		}) {
			candidates = append(candidates, issue.ID)
		}
	}
	return candidates
}

// parseBlockedListing parses `bd blocked --json`, an array of issues with
// blocked_by, into the set of listed IDs minus the rows whose blocked_by
// entries are all external: refs. Main-branch bd lists those through its
// external-dependency policy although their stored flag is clear; released bd
// never lists them, so there the subtraction changes nothing. A row with an
// empty blocked_by is kept: bd lists it for its stored flag.
func parseBlockedListing(out string) (map[string]bool, error) {
	payload, err := bdJSONPayload(out)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID        string   `json:"id"`
		BlockedBy []string `json:"blocked_by"`
	}
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.ID == "" || onlyExternalBlockers(row.BlockedBy) {
			continue
		}
		ids[row.ID] = true
	}
	return ids, nil
}

// onlyExternalBlockers reports whether blockedBy is non-empty and every entry
// is an external: ref.
func onlyExternalBlockers(blockedBy []string) bool {
	if len(blockedBy) == 0 {
		return false
	}
	for _, blocker := range blockedBy {
		if !strings.HasPrefix(blocker, externalRefPrefix) {
			return false
		}
	}
	return true
}

// parseBlockedFlags parses `bd blocked --flags --json`,
// [{"id": ..., "is_blocked": bool}], into the set of IDs whose flag is set.
func parseBlockedFlags(out string) (map[string]bool, error) {
	payload, err := bdJSONPayload(out)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID        string `json:"id"`
		IsBlocked bool   `json:"is_blocked"`
	}
	if err := json.Unmarshal(payload, &rows); err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.ID != "" && row.IsBlocked {
			ids[row.ID] = true
		}
	}
	return ids, nil
}
