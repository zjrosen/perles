//go:build unix

package adapter

import (
	"context"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSnapshotScheduler_ManualRefreshAsBuildStarts: FlushCaches looks for a
// running build twice, before and after it hashes the manifest. A build
// that starts in between forgets the refresh when it starts, so the second
// look marks the refresh as made during that build: if the build fails, the
// backoff stays reset, as if the refresh came right after the failure.
//
// The refresh reads the manifest from a FIFO, which holds it between the two
// looks while the build starts; the build reads a file. POSIX only: Windows
// has no FIFOs.
func TestSnapshotScheduler_ManualRefreshAsBuildStarts(t *testing.T) {
	h := newSchedHarness(t, false)
	require.Equal(t, 1, h.query())
	h.replaceManifest("manifest v2")
	h.b.requestRebuild(false)
	due := h.state().lastEnd.Add(minRebuildGap)
	h.waitTimer(due) // the rebuild waits out pacing

	release, closeRelease := releaseOnCleanup(t)
	building := make(chan struct{})
	h.setExportHook(func(ctx context.Context, n int) error {
		if n != 2 {
			return nil
		}
		close(building)
		select {
		case <-release:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
		return errLocked
	})

	// The manifest becomes a FIFO, which the test opens for writing; the
	// open returns once the refresh has opened it for reading.
	fifo := h.ws.manifest + ".fifo"
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	require.NoError(t, os.Rename(fifo, h.ws.manifest))
	type opened struct {
		w   *os.File
		err error
	}
	writerOpened := make(chan opened, 1)
	go func() {
		w, err := os.OpenFile(h.ws.manifest, os.O_WRONLY, 0)
		writerOpened <- opened{w, err}
	}()
	flushed := make(chan error, 1)
	go func() { flushed <- h.b.FlushCaches(context.Background()) }()

	writer := receiveWithin(t, writerOpened, "the refresh does not read the manifest")
	require.NoError(t, writer.err)
	// Closing the FIFO ends the refresh's read, also when the test fails or
	// hangs: a test blocked on the refresh never reaches its cleanup.
	closeWriter := sync.OnceValue(writer.w.Close)
	hangGuard := time.AfterFunc(5*time.Second, func() { _ = closeWriter() })
	t.Cleanup(func() {
		hangGuard.Stop()
		_ = closeWriter()
	})
	require.False(t, h.state().running, "no build runs at the refresh's first look")

	// The refresh holds the FIFO open; the manifest is a file again for the
	// build, which starts and blocks in its export.
	h.replaceManifest("manifest v2")
	h.advanceTo(due)
	waitClosed(t, building, "the rebuild did not start")
	st := h.state()
	require.True(t, st.running)
	require.False(t, st.manualDuringBuild, "the rebuild forgot the refresh when it started")

	// A write lands: the refresh reads it, after the build hashed the
	// manifest.
	_, err := writer.w.WriteString("manifest v3")
	require.NoError(t, err)
	require.NoError(t, closeWriter())
	require.NoError(t, receiveWithin(t, flushed, "FlushCaches did not return"))
	st = h.state()
	require.True(t, st.manualDuringBuild, "the second look marks the refresh as made during the rebuild")
	require.True(t, st.dirty, "the running build does not cover the change")
	h.replaceManifest("manifest v3")

	closeRelease()
	h.waitBuilt(2)
	st = h.state()
	require.ErrorIs(t, st.lastErr, errLocked)
	require.Zero(t, st.consecutiveFailures, "the failure does not undo the refresh's backoff reset")

	h.rebuildAfterPacing(minRebuildGap) // pacing only, no 5s backoff
	require.NoError(t, h.state().lastErr)
	require.Equal(t, manifestHashOf("manifest v3"), h.state().installedHash)
	require.Equal(t, 2, h.query())
}
