package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Fixtures from the same edge-case workspace, built twice with the same
// commands and IDs: once with bd 1.3.2 main@6ebaacba8 (the "-main" files, the
// only build with blocked --flags) and once with bd v1.3.2 built from the
// release tag. The workspace has, besides an open blocker fx-blocker:
//   - fx-blocked (blocks), fx-cond (conditional-blocks) and fx-dependent (a
//     blocks edge added from its blocker fx-incoming, which has only that
//     incoming edge): blocked;
//   - fx-epic (blocks) and its child fx-epic.1: blocked down parent-child;
//   - fx-on-shipped: blocked by fx-shipped in the custom done status shipped;
//   - fx-waits: a waits-for gate on fx-spawner, whose child fx-spawner.1 is open;
//   - fx-ext: blocked only by external:other:cap, stored flag clear;
//   - fx-mixed: blocked by external:other:cap and fx-blocker;
//   - fx-on-done (closed blocker fx-done), fx-pinned (pinned, blocks),
//     fx-related (related) and fx-found (discovered-from): not blocked.
//
// bd --readonly blocked --flags --raw --json over every ID (the stored
// column) sets exactly blockedFixtureTruth, on main.
const (
	// blockedExportFixture is `bd --readonly export --all` (main).
	blockedExportFixture = "testdata/bd_blocked_export_v1.3.2-main.jsonl"
	// blockedListingMainFixture is `bd --readonly blocked --json` (main): it
	// also lists fx-ext through the external-dependency policy.
	blockedListingMainFixture = "testdata/bd_blocked_v1.3.2-main.json"
	// blockedListingReleaseFixture is `bd --readonly blocked --json` (v1.3.2
	// release): no fx-ext, and fx-mixed names only its local blocker.
	blockedListingReleaseFixture = "testdata/bd_blocked_v1.3.2.json"
	// blockedFlagsFixture is `bd --readonly blocked --flags --raw --json`
	// (main) with blockedFixtureCandidates on stdin.
	blockedFlagsFixture = "testdata/bd_blocked_flags_raw_v1.3.2-main.json"
)

// blockedFixtureTruth is the workspace's stored is_blocked column.
var blockedFixtureTruth = map[string]bool{
	"fx-blocked": true, "fx-cond": true, "fx-dependent": true, "fx-epic": true,
	"fx-epic.1": true, "fx-mixed": true, "fx-on-shipped": true, "fx-waits": true,
}

// blockedFixtureCandidates are the candidates of the export, in its order.
var blockedFixtureCandidates = []string{
	"fx-dependent", "fx-waits", "fx-spawner.1", "fx-mixed", "fx-ext", "fx-on-done",
	"fx-on-shipped", "fx-epic.1", "fx-epic", "fx-cond", "fx-blocked",
}

// releasedFlagsRefusal is bd v1.3.2's answer to blockedFlagsArgs: exit 1,
// nothing on stdout.
var releasedFlagsRefusal = bdFailed(1, "", "Error: unknown flag: --flags\n")

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: test fixture
	require.NoError(t, err)
	return string(data)
}

func decodeBlockedFixture(t *testing.T) []ExportIssue {
	t.Helper()
	issues, err := DecodeExport(strings.NewReader(readFixture(t, blockedExportFixture)))
	require.NoError(t, err)
	return issues
}

// bdPrinted is a successful bd run that printed stdout.
func bdPrinted(stdout string) bdOutput { return bdOutput{Stdout: stdout} }

func isFlagsRequest(req BDRequest) bool { return slices.Contains(req.Args, "--flags") }

// bdBlocked answers the --flags read with flags and the listing with listing.
func bdBlocked(flags, listing bdOutput) func(BDRequest) bdOutput {
	return func(req BDRequest) bdOutput {
		if isFlagsRequest(req) {
			return flags
		}
		return listing
	}
}

// countRequests returns how many requests asked for --flags and how many for
// the listing.
func countRequests(requests []BDRequest) (flags, listing int) {
	for _, req := range requests {
		if isFlagsRequest(req) {
			flags++
		} else {
			listing++
		}
	}
	return flags, listing
}

func TestParseBlockedListing_Fixtures(t *testing.T) {
	t.Run("main listing minus the external-only row", func(t *testing.T) {
		out := readFixture(t, blockedListingMainFixture)
		require.Contains(t, out, `"id": "fx-ext"`, "main lists the external-only issue")

		ids, err := parseBlockedListing(out)
		require.NoError(t, err)
		require.Equal(t, blockedFixtureTruth, ids, "fx-ext is removed, fx-mixed (external and local blockers) is kept")
	})

	t.Run("released listing comes back unchanged", func(t *testing.T) {
		out := readFixture(t, blockedListingReleaseFixture)
		var rows []struct {
			ID string `json:"id"`
		}
		payload, err := bdJSONPayload(out)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(payload, &rows))
		listed := map[string]bool{}
		for _, row := range rows {
			listed[row.ID] = true
		}

		ids, err := parseBlockedListing(out)
		require.NoError(t, err)
		require.Equal(t, listed, ids, "released bd lists no external-only row")
		require.Equal(t, blockedFixtureTruth, ids)
	})
}

func TestParseBlockedListing(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want map[string]bool
	}{
		{
			name: "only external blockers are removed",
			out: `[{"id":"a-1","blocked_by":["external:p:cap"]},
				{"id":"a-2","blocked_by":["external:p:cap","external:q:other"]},
				{"id":"a-3","blocked_by":["external:p:cap","a-9"]},
				{"id":"a-4","blocked_by":["a-9"]}]`,
			want: map[string]bool{"a-3": true, "a-4": true},
		},
		{
			name: "a row without named blockers is kept",
			out:  `[{"id":"a-1","blocked_by":[]},{"id":"a-2"},{"id":"a-3","blocked_by":null}]`,
			want: map[string]bool{"a-1": true, "a-2": true, "a-3": true},
		},
		{
			name: "external is a prefix, not a substring",
			out:  `[{"id":"a-1","blocked_by":["proj-external:x"]}]`,
			want: map[string]bool{"a-1": true},
		},
		{name: "rows without an id are skipped", out: `[{"blocked_by":["a-9"]},{"id":"a-1","blocked_by":["a-9"]}]`, want: map[string]bool{"a-1": true}},
		{name: "empty listing", out: "[]\n", want: map[string]bool{}},
		{name: "null listing", out: "null\n", want: map[string]bool{}},
		{
			name: "enveloped",
			out:  `{"schema_version":1,"data":[{"id":"a-1","blocked_by":["a-9"]}]}`,
			want: map[string]bool{"a-1": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ids, err := parseBlockedListing(tt.out)
			require.NoError(t, err)
			require.Equal(t, tt.want, ids)
		})
	}

	for name, out := range map[string]string{
		"empty output": "",
		"not JSON":     "No blocked issues\n",
		"an object":    `{"id":"a-1"}`,
	} {
		t.Run("error: "+name, func(t *testing.T) {
			_, err := parseBlockedListing(out)
			require.Error(t, err)
		})
	}
}

func TestParseBlockedFlags(t *testing.T) {
	ids, err := parseBlockedFlags(readFixture(t, blockedFlagsFixture))
	require.NoError(t, err)
	require.Equal(t, blockedFixtureTruth, ids, "the flags bd reports set, no others")

	ids, err = parseBlockedFlags(`{"schema_version":1,"data":[{"id":"a-1","is_blocked":true},{"id":"a-2","is_blocked":false},{"id":"","is_blocked":true}]}`)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"a-1": true}, ids)

	ids, err = parseBlockedFlags("[]")
	require.NoError(t, err)
	require.Empty(t, ids)

	_, err = parseBlockedFlags(`[{"id":"a-1","is_blocked":"yes"}]`)
	require.Error(t, err)
	_, err = parseBlockedFlags("")
	require.Error(t, err)
}

func TestBlockedFlagCandidates_Fixture(t *testing.T) {
	candidates := blockedFlagCandidates(decodeBlockedFixture(t))
	require.Equal(t, blockedFixtureCandidates, candidates)
	for _, id := range []string{"fx-pinned", "fx-done", "fx-related", "fx-found", "fx-incoming", "fx-blocker", "fx-spawner", "fx-shipped"} {
		require.NotContains(t, candidates, id)
	}
	for id := range blockedFixtureTruth {
		require.Contains(t, candidates, id, "every stored flag is on a candidate")
	}
}

func TestBlockedFlagCandidates(t *testing.T) {
	dep := func(depType string) []ExportDependency {
		return []ExportDependency{{DependsOnID: "t-1", Type: depType}}
	}
	issues := []ExportIssue{
		{ID: "blocks", Status: "open", Dependencies: dep("blocks")},
		{ID: "conditional", Status: "in_progress", Dependencies: dep("conditional-blocks")},
		{ID: "waits", Status: "open", Dependencies: dep("waits-for")},
		{ID: "child", Status: "open", Dependencies: dep("parent-child")},
		{ID: "external", Status: "open", Dependencies: []ExportDependency{{DependsOnID: "external:p:cap", Type: "blocks"}}},
		{ID: "custom-status", Status: "in_review", Dependencies: dep("blocks")},
		{ID: "blocked-status", Status: "blocked", Dependencies: dep("blocks")},
		{ID: "mixed-types", Status: "open", Dependencies: []ExportDependency{{DependsOnID: "t-1", Type: "related"}, {DependsOnID: "t-2", Type: "waits-for"}}},
		{ID: "closed", Status: "closed", Dependencies: dep("blocks")},
		{ID: "pinned", Status: "pinned", Dependencies: dep("blocks")},
		{ID: "related", Status: "open", Dependencies: dep("related")},
		{ID: "discovered", Status: "open", Dependencies: dep("discovered-from")},
		{ID: "tracks", Status: "open", Dependencies: dep("tracks")},
		{ID: "no-edges", Status: "open"},
		// The target of the other edges: only incoming edges.
		{ID: "t-1", Status: "open"},
	}
	require.Equal(t, []string{"blocks", "conditional", "waits", "child", "external", "custom-status", "blocked-status", "mixed-types"},
		blockedFlagCandidates(issues))
	require.Empty(t, blockedFlagCandidates(nil))
}

// requireBlockedRequest asserts the shape every blocked read shares.
func requireBlockedRequest(t *testing.T, req BDRequest) {
	t.Helper()
	require.Equal(t, snapshotSourceTimeout, req.Timeout)
	require.Empty(t, req.Env)
	if isFlagsRequest(req) {
		require.Equal(t, []string{"--readonly", "blocked", "--flags", "--raw", "--json"}, req.Args, "--flags before --raw")
	} else {
		require.Equal(t, []string{"--readonly", "blocked", "--json"}, req.Args)
		require.Empty(t, req.Stdin)
	}
}

func TestBlockedFlagSource_NoCandidatesRunsNoBD(t *testing.T) {
	run, requests := sourceRunner(t, func(BDRequest) bdOutput {
		t.Fatal("bd must not run without candidates")
		return bdOutput{}
	})
	source := NewBlockedFlagSource(run)
	for _, issues := range [][]ExportIssue{
		nil,
		{{ID: "a-1", Status: "open"}},
		{{ID: "a-1", Status: "open", Dependencies: []ExportDependency{{DependsOnID: "a-2", Type: "related"}}}},
		{{ID: "a-1", Status: "closed", Dependencies: []ExportDependency{{DependsOnID: "a-2", Type: "blocks"}}}},
	} {
		ids, err := source.ReadBlockedIDs(context.Background(), issues)
		require.NoError(t, err)
		require.NotNil(t, ids)
		require.Empty(t, ids)
	}
	require.Empty(t, *requests)
}

// TestBlockedFlagSource_FirstBuildWithCandidatesProbes: builds without
// candidates leave support untested, so the first build with candidates is
// the one that asks for --flags.
func TestBlockedFlagSource_FirstBuildWithCandidatesProbes(t *testing.T) {
	run, requests := sourceRunner(t, bdBlocked(releasedFlagsRefusal, bdPrinted(readFixture(t, blockedListingReleaseFixture))))
	source := NewBlockedFlagSource(run)

	_, err := source.ReadBlockedIDs(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, *requests)

	ids, err := source.ReadBlockedIDs(context.Background(), decodeBlockedFixture(t))
	require.NoError(t, err)
	require.Equal(t, blockedFixtureTruth, ids)
	require.Len(t, *requests, 2)
	require.True(t, isFlagsRequest((*requests)[0]), "the probe comes first")
	require.False(t, isFlagsRequest((*requests)[1]))
}

func TestBlockedFlagSource_FlagsSupported(t *testing.T) {
	run, requests := sourceRunner(t, bdBlocked(bdPrinted(readFixture(t, blockedFlagsFixture)), bdOutput{}))
	source := NewBlockedFlagSource(run)
	issues := decodeBlockedFixture(t)

	for range 3 {
		ids, err := source.ReadBlockedIDs(context.Background(), issues)
		require.NoError(t, err)
		require.Equal(t, blockedFixtureTruth, ids)
	}

	flags, listing := countRequests(*requests)
	require.Equal(t, 3, flags, "a supported --flags is used for every build")
	require.Zero(t, listing, "the listing is never needed")
	for _, req := range *requests {
		requireBlockedRequest(t, req)
		require.Equal(t, strings.Join(blockedFixtureCandidates, "\n")+"\n", req.Stdin, "only the candidate IDs, one per line")
	}
	require.Equal(t, blockedFlagsSupported, source.supportState())
}

// TestBlockedFlagSource_UnsupportedIsCached: a bd without --flags (or
// without --raw) gets one probe; that build and every later one use the
// listing.
func TestBlockedFlagSource_UnsupportedIsCached(t *testing.T) {
	for _, refusal := range []string{"Error: unknown flag: --flags\n", "Error: unknown flag: --raw\n"} {
		t.Run(strings.TrimSpace(refusal), func(t *testing.T) {
			listing := readFixture(t, blockedListingReleaseFixture)
			run, requests := sourceRunner(t, bdBlocked(bdFailed(1, "", refusal), bdPrinted(listing)))
			source := NewBlockedFlagSource(run)
			issues := decodeBlockedFixture(t)

			for build := 1; build <= 4; build++ {
				ids, err := source.ReadBlockedIDs(context.Background(), issues)
				require.NoError(t, err, "build %d", build)
				require.Equal(t, blockedFixtureTruth, ids, "build %d", build)
			}

			flags, listings := countRequests(*requests)
			require.Equal(t, 1, flags, "the failing probe runs once per source")
			require.Equal(t, 4, listings, "the probing build and every later one use the listing")
			require.True(t, isFlagsRequest((*requests)[0]))
			for _, req := range *requests {
				requireBlockedRequest(t, req)
			}
			require.Equal(t, blockedFlagsUnsupported, source.supportState())
		})
	}
}

// TestBlockedFlagSource_ListingParity: on the listing path the blocked IDs
// are the listing's IDs minus external-only rows, which on this workspace is
// exactly the stored column, for main's listing and the released one.
func TestBlockedFlagSource_ListingParity(t *testing.T) {
	for _, fixture := range []string{blockedListingMainFixture, blockedListingReleaseFixture} {
		t.Run(fixture, func(t *testing.T) {
			run, _ := sourceRunner(t, bdBlocked(releasedFlagsRefusal, bdPrinted(readFixture(t, fixture))))
			ids, err := NewBlockedFlagSource(run).ReadBlockedIDs(context.Background(), decodeBlockedFixture(t))
			require.NoError(t, err)
			require.Equal(t, blockedFixtureTruth, ids)
			require.NotContains(t, ids, "fx-ext")
		})
	}
}

// TestBlockedFlagSource_OtherFailuresFailTheBuild: only the exact --flags or
// --raw token means unsupported. Anything else is the build's error, nothing
// is cached, and the next build probes again.
func TestBlockedFlagSource_OtherFailuresFailTheBuild(t *testing.T) {
	tests := []struct {
		name       string
		flagsOut   bdOutput
		wantPrefix string
	}{
		{
			name:       "near-miss flag name",
			flagsOut:   bdFailed(1, "", "Error: unknown flag: --flagsx\n"),
			wantPrefix: "bd blocked failed: unknown flag: --flagsx",
		},
		{
			name:       "timeout",
			flagsOut:   bdOutput{ExitCode: -1, Err: errors.New("signal: killed"), TimedOut: true},
			wantPrefix: "bd blocked timed out after 30s: " + hintTimeout,
		},
		{
			name:       "migration refusal",
			flagsOut:   bdFailed(1, "", "Error: database schema is at v45 but this binary expects v46; run 'bd migrate schema'\n"),
			wantPrefix: "bd blocked failed: " + hintMigrate + " — ",
		},
		{
			name:       "unknown shorthand flag",
			flagsOut:   bdFailed(1, "", "Error: unknown shorthand flag: 'f' in -f\n"),
			wantPrefix: "bd blocked failed: unknown shorthand flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagsOut := tt.flagsOut
			run, requests := sourceRunner(t, func(req BDRequest) bdOutput {
				require.True(t, isFlagsRequest(req), "a failed probe never falls back to the listing")
				return flagsOut
			})
			source := NewBlockedFlagSource(run)
			issues := decodeBlockedFixture(t)

			ids, err := source.ReadBlockedIDs(context.Background(), issues)
			require.Nil(t, ids)
			bdErr, ok := errors.AsType[*bdError](err)
			require.True(t, ok, "want the translated *bdError, got %v", err)
			require.Equal(t, "blocked", bdErr.Command)
			require.True(t, strings.HasPrefix(err.Error(), tt.wantPrefix), "got %q, want prefix %q", err.Error(), tt.wantPrefix)
			require.Equal(t, blockedFlagsUnknown, source.supportState(), "nothing is cached")

			// The next build asks for --flags again, and its success is cached.
			flagsOut = bdPrinted(readFixture(t, blockedFlagsFixture))
			ids, err = source.ReadBlockedIDs(context.Background(), issues)
			require.NoError(t, err)
			require.Equal(t, blockedFixtureTruth, ids)
			require.Len(t, *requests, 2)
			require.Equal(t, blockedFlagsSupported, source.supportState())
		})
	}
}

func TestBlockedFlagSource_MalformedFlagsOutput(t *testing.T) {
	run, requests := sourceRunner(t, bdBlocked(bdPrinted(`[{"id":"fx-blocked","is_blocked":1}]`), bdOutput{}))
	source := NewBlockedFlagSource(run)

	ids, err := source.ReadBlockedIDs(context.Background(), decodeBlockedFixture(t))
	require.Nil(t, ids)
	require.ErrorContains(t, err, "parsing bd blocked --flags output")
	require.Len(t, *requests, 1)
	require.Equal(t, blockedFlagsUnknown, source.supportState())
}

func TestBlockedFlagSource_ListingFailures(t *testing.T) {
	t.Run("bd failure is the translated *bdError", func(t *testing.T) {
		run, _ := sourceRunner(t, bdBlocked(releasedFlagsRefusal, bdFailed(1, "", "Error: schema version mismatch: database is at v80, binary knows up to v74 (6 migrations ahead)\n")))
		ids, err := NewBlockedFlagSource(run).ReadBlockedIDs(context.Background(), decodeBlockedFixture(t))
		require.Nil(t, ids)
		bdErr, ok := errors.AsType[*bdError](err)
		require.True(t, ok, "got %v", err)
		require.Equal(t, "blocked", bdErr.Command)
		require.True(t, strings.HasPrefix(err.Error(), "bd blocked failed: "+hintUpgradeBD+" — "), err.Error())
	})

	t.Run("malformed output", func(t *testing.T) {
		run, _ := sourceRunner(t, bdBlocked(releasedFlagsRefusal, bdPrinted("No blocked issues\n")))
		source := NewBlockedFlagSource(run)
		ids, err := source.ReadBlockedIDs(context.Background(), decodeBlockedFixture(t))
		require.Nil(t, ids)
		require.ErrorContains(t, err, "parsing bd blocked output")
		require.Equal(t, blockedFlagsUnsupported, source.supportState(), "the probe's answer stands")
	})
}

// TestBlockedFlagSource_SupportLostAfterwards: if bd is replaced by one
// without --flags after a supported build, the refusal is cached as well and
// the build uses the listing.
func TestBlockedFlagSource_SupportLostAfterwards(t *testing.T) {
	flagsOut := bdPrinted(readFixture(t, blockedFlagsFixture))
	run, requests := sourceRunner(t, func(req BDRequest) bdOutput {
		if isFlagsRequest(req) {
			return flagsOut
		}
		return bdPrinted(readFixture(t, blockedListingReleaseFixture))
	})
	source := NewBlockedFlagSource(run)
	issues := decodeBlockedFixture(t)

	_, err := source.ReadBlockedIDs(context.Background(), issues)
	require.NoError(t, err)
	require.Equal(t, blockedFlagsSupported, source.supportState())

	flagsOut = releasedFlagsRefusal
	for range 2 {
		ids, err := source.ReadBlockedIDs(context.Background(), issues)
		require.NoError(t, err)
		require.Equal(t, blockedFixtureTruth, ids)
	}
	flags, listing := countRequests(*requests)
	require.Equal(t, 2, flags)
	require.Equal(t, 2, listing)
	require.Equal(t, blockedFlagsUnsupported, source.supportState())
}

// TestBlockedFlagSource_ConcurrentFirstBuilds: builds racing on an untested
// source (the scheduler and a lazy first build) send a released bd one probe.
func TestBlockedFlagSource_ConcurrentFirstBuilds(t *testing.T) {
	listing := readFixture(t, blockedListingReleaseFixture)
	var flagsCalls, listingCalls atomic.Int32
	run := func(_ context.Context, req BDRequest) (string, error) {
		if isFlagsRequest(req) {
			flagsCalls.Add(1)
			time.Sleep(20 * time.Millisecond) // keep the other builds waiting on the probe
			return "", newBDError(req.Args, releasedFlagsRefusal, req.Timeout)
		}
		listingCalls.Add(1)
		return listing, nil
	}
	source := NewBlockedFlagSource(run)
	issues := decodeBlockedFixture(t)

	const builds = 8
	var wg sync.WaitGroup
	results := make([]map[string]bool, builds)
	errs := make([]error, builds)
	for i := range builds {
		wg.Go(func() {
			results[i], errs[i] = source.ReadBlockedIDs(context.Background(), issues)
		})
	}
	wg.Wait()

	for i := range builds {
		require.NoError(t, errs[i])
		require.Equal(t, blockedFixtureTruth, results[i])
	}
	require.Equal(t, int32(1), flagsCalls.Load())
	require.Equal(t, int32(builds), listingCalls.Load())
}

// TestBlockedFlagSource_ProbeWaitEndsWithContext: a build waiting for
// another build's probe gives up when its context ends.
func TestBlockedFlagSource_ProbeWaitEndsWithContext(t *testing.T) {
	probing := make(chan struct{})
	release := make(chan struct{})
	run := func(context.Context, BDRequest) (string, error) {
		close(probing)
		<-release
		return readFixture(t, blockedFlagsFixture), nil
	}
	source := NewBlockedFlagSource(run)
	issues := decodeBlockedFixture(t)

	first := make(chan error, 1)
	go func() {
		_, err := source.ReadBlockedIDs(context.Background(), issues)
		first <- err
	}()
	<-probing

	ctx, cancel := context.WithCancelCause(context.Background())
	errClosed := errors.New("backend closed")
	cancel(errClosed)
	ids, err := source.ReadBlockedIDs(ctx, issues)
	require.Nil(t, ids)
	require.ErrorIs(t, err, errClosed)

	close(release)
	require.NoError(t, <-first)
	require.Equal(t, blockedFlagsSupported, source.supportState())
}

func TestBlockedFlagSource_PassesContext(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "caller")
	var seen []any
	run := func(ctx context.Context, req BDRequest) (string, error) {
		seen = append(seen, ctx.Value(ctxKey{}))
		if isFlagsRequest(req) {
			return "", newBDError(req.Args, releasedFlagsRefusal, req.Timeout)
		}
		return readFixture(t, blockedListingReleaseFixture), nil
	}
	_, err := NewBlockedFlagSource(run).ReadBlockedIDs(ctx, decodeBlockedFixture(t))
	require.NoError(t, err)
	require.Equal(t, []any{"caller", "caller"}, seen, "the caller's context reaches both bd reads")
}
