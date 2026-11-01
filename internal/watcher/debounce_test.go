package watcher

// Debounce timing tests. Each runs the watcher's loop in a testing/synctest
// bubble, whose fake clock moves only when every goroutine in it is blocked,
// and feeds the loop events from a channel in place of fsnotify (whose
// reader goroutine sits in a system call, which would stop the fake clock).
// The publish times they assert are therefore exact, and no test sleeps.

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/pubsub"
)

// publishTimes runs the watcher's loop, configured with cfg, sends it an
// event on a relevant file at each of offsets (from the start, ascending)
// and returns the offsets at which it published DBChanged. It must run in a
// synctest bubble.
func publishTimes(t *testing.T, cfg Config, offsets []time.Duration) []time.Duration {
	t.Helper()
	start := time.Now()
	events := make(chan fsnotify.Event)
	// The fields New sets, with the fsnotify watcher replaced by the
	// channels the loop reads.
	w := &Watcher{
		fsWatcher:     &fsnotify.Watcher{Events: events, Errors: make(chan error)},
		dbPath:        cfg.DBPath,
		relevantFiles: cfg.RelevantFiles,
		debounce:      cfg.DebounceDur,
		maxWait:       cfg.MaxWait,
		done:          make(chan struct{}),
		broker:        pubsub.NewBroker[WatcherEvent](),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := w.broker.Subscribe(ctx)
	looping := make(chan struct{})
	go func() {
		defer close(looping)
		w.loop()
	}()
	defer func() {
		close(w.done)
		<-looping
		w.broker.Close()
	}()

	event := fsnotify.Event{Name: "/workspace/" + cfg.RelevantFiles[0], Op: fsnotify.Create}
	for _, at := range offsets {
		time.Sleep(time.Until(start.Add(at)))
		events <- event
	}
	time.Sleep(time.Hour) // long past every timer

	var published []time.Duration
	for {
		select {
		case e := <-sub:
			require.Equal(t, DBChanged, e.Payload.Type)
			published = append(published, e.Timestamp.Sub(start))
		default:
			return published
		}
	}
}

// sustainedEvents is an event every 300ms for 8.7s (30 events), so never a
// pause as long as a 500ms debounce: bd rewrites the manifest throughout
// each of its commands, and agents running one about every 0.8s leave
// shorter gaps than that.
func sustainedEvents() []time.Duration {
	offsets := make([]time.Duration, 30)
	for i := range offsets {
		offsets[i] = time.Duration(i) * 300 * time.Millisecond
	}
	return offsets
}

// manifestConfig is the snapshot backend's manifest watcher, with maxWait.
func manifestConfig(maxWait time.Duration) Config {
	return Config{
		DBPath:        "/workspace/manifest",
		RelevantFiles: []string{"manifest"},
		DebounceDur:   500 * time.Millisecond,
		MaxWait:       maxWait,
	}
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

// TestWatcher_MaxWait_SustainedEvents: with MaxWait set, a stream of events
// that never pauses for the debounce is still published every MaxWait: 2s
// after the stream's first event, then 2s after the first event following
// each publish. The events after the last of those are published a
// debounce after the stream ends.
func TestWatcher_MaxWait_SustainedEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		got := publishTimes(t, manifestConfig(2*time.Second), sustainedEvents())
		require.Equal(t, []time.Duration{ms(2000), ms(4100), ms(6200), ms(8300), ms(9200)}, got)
	})
}

// TestWatcher_DefaultHasNoMaxWait: without MaxWait, the default and the
// configuration of every watcher but the snapshot backend's (server mode's
// last-touched sentinel, beads_rust's database files), the same stream is
// published once, a debounce after it ends, as before MaxWait existed.
func TestWatcher_DefaultHasNoMaxWait(t *testing.T) {
	configs := map[string]Config{
		"manifest, no max wait": manifestConfig(0),
		"server mode": {
			DBPath:        "/workspace/.beads/dolt",
			RelevantFiles: []string{"last-touched"},
			DebounceDur:   500 * time.Millisecond,
		},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				require.Equal(t, []time.Duration{ms(9200)}, publishTimes(t, cfg, sustainedEvents()))
			})
		})
	}
	t.Run("beads_rust", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cfg := Config{
				DBPath:        "/workspace/beads.db",
				RelevantFiles: []string{"beads.db", "beads.db-wal"},
				DebounceDur:   100 * time.Millisecond,
			}
			// Events 50ms apart for 4.95s, then a debounce.
			offsets := make([]time.Duration, 100)
			for i := range offsets {
				offsets[i] = time.Duration(i) * 50 * time.Millisecond
			}
			require.Equal(t, []time.Duration{ms(5050)}, publishTimes(t, cfg, offsets))
		})
	})
}

// TestWatcher_MaxWait_BurstsThatPauseUseTheDebounce: with MaxWait set,
// bursts that pause for the debounce are published a debounce after their
// last event, as without it, and each burst gets its own maximum wait: the
// third event, 1.9s after the first, is published at 2.4s, not when the
// first burst's maximum wait would have ended (2s).
func TestWatcher_MaxWait_BurstsThatPauseUseTheDebounce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		got := publishTimes(t, manifestConfig(2*time.Second), []time.Duration{0, ms(300), ms(1900)})
		require.Equal(t, []time.Duration{ms(800), ms(2400)}, got)
	})
}
