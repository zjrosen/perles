// Package zonetest provides test helpers for bubblezone mouse zones.
package zonetest

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	zone "github.com/lrstanley/bubblezone"
	"github.com/stretchr/testify/require"
)

// barrierSeq makes every barrier zone ID unique, so a barrier can never be
// satisfied by a zone left over from an earlier call.
var barrierSeq atomic.Int64

// ScanAndWait scans view, an unscanned render (the string a View method hands
// to zone.Scan), with the global bubblezone manager and waits until the
// manager has stored every zone view marks. After it returns, zone.Get
// reports the bounds from view for each of those zones.
//
// zone.Scan returns before its zones are stored: it queues them for a
// background worker, so zone.Get right after View() can still return bounds
// from an earlier render. The worker stores zones in the order they were
// queued, so ScanAndWait appends one more marked cell to view (after
// everything else, so no other zone moves) and waits for that zone to appear.
//
// Zones from earlier renders that view does not mark can stay registered:
// bubblezone drops them asynchronously, and not at all while renders share a
// clock tick (zone.Scan keys each render by time.Now().Nanosecond(), which on
// Windows only advances once per clock tick). A click handler can then match
// such a stale zone, for example a pane's action from before another pane was
// maximized. Pass the IDs of zones that may be stale: ScanAndWait first lets
// the worker store everything already queued, so no pending zone is stored
// after it is cleared, then removes them before scanning view.
func ScanAndWait(tb testing.TB, view string, stale ...string) {
	tb.Helper()

	if len(stale) > 0 {
		scanBarrier(tb, "")
		for _, id := range stale {
			zone.Clear(id)
		}
	}
	scanBarrier(tb, view)
}

// scanBarrier scans view followed by a unique barrier zone and waits until the
// worker has stored the barrier, which means it has also stored every zone
// queued before it.
func scanBarrier(tb testing.TB, view string) {
	tb.Helper()

	barrierID := fmt.Sprintf("zonetest-barrier-%d", barrierSeq.Add(1))
	_ = zone.Scan(view + zone.Mark(barrierID, " "))

	require.Eventually(tb, func() bool {
		return zone.Get(barrierID) != nil
	}, 10*time.Second, time.Millisecond, "bubblezone did not store the zones of the scanned view")
}
