package watcher

// Debounce timing tests. Each runs the watcher's loop in a testing/synctest
// bubble, whose fake clock moves only when every goroutine in it is blocked,
// and feeds the loop events and errors from channels in place of fsnotify
// (whose reader goroutine sits in a system call, which would stop the fake
// clock).
// The publish times they assert are therefore exact, and no test sleeps.

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/pubsub"
)

// loopRun is a watcher's loop running in a synctest bubble, reading the
// channels it is sent events and errors on in place of fsnotify's, and a
// subscription to its broker.
type loopRun struct {
	start   time.Time
	cfg     Config
	events  chan fsnotify.Event
	errs    chan error
	w       *Watcher
	sub     <-chan pubsub.Event[WatcherEvent]
	looping chan struct{}
	// stop is Stop without closing the fsnotify watcher, which is not a
	// real one. It waits for the loop to return before closing the broker,
	// so a publish the loop makes while it stops still reaches sub. The
	// test's end calls it too.
	stop func()
}

// publication is one event the loop published, at its offset from the
// start.
type publication struct {
	at  time.Duration
	typ WatcherEventType
	err error
}

// startLoop starts the loop of a watcher configured with cfg, to which the
// test sends up to inputs events and errors. The loop publishes at most once
// per input, so a subscription buffer of that size holds every publish. It
// must run in a synctest bubble.
func startLoop(t *testing.T, cfg Config, inputs int) *loopRun {
	t.Helper()
	r := &loopRun{
		start:   time.Now(),
		cfg:     cfg,
		events:  make(chan fsnotify.Event),
		errs:    make(chan error),
		looping: make(chan struct{}),
	}
	r.w = newWatcher(cfg, &fsnotify.Watcher{Events: r.events, Errors: r.errs})
	r.w.broker = pubsub.NewBrokerWithBuffer[WatcherEvent](inputs) // not the default 64
	r.sub = r.w.broker.Subscribe(t.Context())
	go func() {
		defer close(r.looping)
		r.w.loop()
	}()
	r.stop = sync.OnceFunc(func() {
		close(r.w.done)
		<-r.looping
		r.w.broker.Close()
	})
	t.Cleanup(r.stop)
	return r
}

// sleepUntil sleeps until offset at from the start.
func (r *loopRun) sleepUntil(at time.Duration) {
	time.Sleep(time.Until(r.start.Add(at)))
}

// sendAt sends the loop event at offset at.
func (r *loopRun) sendAt(at time.Duration, event fsnotify.Event) {
	r.sleepUntil(at)
	r.events <- event
}

// errorAt sends the loop err at offset at.
func (r *loopRun) errorAt(at time.Duration, err error) {
	r.sleepUntil(at)
	r.errs <- err
}

// relevant is an event on the first relevant file: a Create, as bd's
// rename-replaced manifest is reported.
func (r *loopRun) relevant() fsnotify.Event {
	return fsnotify.Event{Name: "/workspace/" + r.cfg.RelevantFiles[0], Op: fsnotify.Create}
}

// published sleeps until long past every timer, then returns what the loop
// has published.
func (r *loopRun) published() []publication {
	time.Sleep(time.Hour)
	var got []publication
	for {
		select {
		case e, ok := <-r.sub:
			if !ok {
				return got
			}
			got = append(got, publication{at: e.Timestamp.Sub(r.start), typ: e.Payload.Type, err: e.Payload.Error})
		default:
			return got
		}
	}
}

// publishTimes runs the watcher's loop, configured with cfg, sends it an
// event on a relevant file at each of offsets (from the start, ascending)
// and returns the offsets at which it published DBChanged. It must run in a
// synctest bubble.
func publishTimes(t *testing.T, cfg Config, offsets []time.Duration) []time.Duration {
	t.Helper()
	r := startLoop(t, cfg, len(offsets))
	for _, at := range offsets {
		r.sendAt(at, r.relevant())
	}
	var times []time.Duration
	for _, p := range r.published() {
		require.Equal(t, DBChanged, p.typ)
		times = append(times, p.at)
	}
	return times
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

// TestWatcher_MaxWait_ErrorMidStream: an fsnotify error during a stream
// that never pauses is published at once as WatcherError, and leaves the
// stream's DBChanged times as in TestWatcher_MaxWait_SustainedEvents. It
// arrives at 1.95s, after the last event before the first maximum wait
// ends, so that wait still reports the events pending then: the error
// neither drops them nor restarts the wait.
func TestWatcher_MaxWait_ErrorMidStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		offsets := sustainedEvents()
		r := startLoop(t, manifestConfig(2*time.Second), len(offsets)+1)
		for _, at := range offsets {
			r.sendAt(at, r.relevant())
			if at == ms(1800) {
				r.errorAt(ms(1950), fsnotify.ErrEventOverflow)
			}
		}
		changed := func(at time.Duration) publication { return publication{at: at, typ: DBChanged} }
		require.Equal(t, []publication{
			{at: ms(1950), typ: WatcherError, err: fsnotify.ErrEventOverflow},
			changed(ms(2000)), changed(ms(4100)), changed(ms(6200)), changed(ms(8300)), changed(ms(9200)),
		}, r.published())
	})
}

// TestWatcher_MaxWait_IrrelevantEventsDoNotStartIt: events the watcher
// ignores (another file in the directory, a chmod of the manifest) start no
// maximum wait. Only the relevant event at 1.9s does, so it is published a
// debounce later, at 2.4s, not when a wait started by the first ignored
// event would end (2s).
func TestWatcher_MaxWait_IrrelevantEventsDoNotStartIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const ignored = 7 // at 0, 300ms, ..., 1.8s
		r := startLoop(t, manifestConfig(2*time.Second), 2*ignored+1)
		for i := range ignored {
			at := time.Duration(i) * 300 * time.Millisecond
			r.sendAt(at, fsnotify.Event{Name: "/workspace/manifest.tmp", Op: fsnotify.Create})
			r.sendAt(at, fsnotify.Event{Name: "/workspace/manifest", Op: fsnotify.Chmod})
		}
		r.sendAt(ms(1900), r.relevant())
		require.Equal(t, []publication{{at: ms(2400), typ: DBChanged}}, r.published())
	})
}

// TestWatcher_MaxWait_StopDropsPendingBurst: stopping the watcher while a
// burst is pending, with its debounce and its maximum wait running,
// publishes nothing: the loop returns at once, and neither timer fires.
func TestWatcher_MaxWait_StopDropsPendingBurst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := startLoop(t, manifestConfig(2*time.Second), 3)
		for _, at := range []time.Duration{0, ms(300), ms(600)} {
			r.sendAt(at, r.relevant())
		}
		r.sleepUntil(ms(700)) // the debounce would fire at 1.1s, the maximum wait at 2s
		r.stop()
		require.Empty(t, r.published())
	})
}
