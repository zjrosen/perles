// Package watcher provides file system watching with debouncing for the beads database.
package watcher

import (
	"fmt"
	"path/filepath"
	"time"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/pubsub"

	"github.com/fsnotify/fsnotify"
)

// WatcherEventType identifies the kind of watcher event.
type WatcherEventType string

const (
	// DBChanged is emitted when the database file changes (after debounce).
	DBChanged WatcherEventType = "db_changed"
	// WatcherError is emitted when the watcher encounters an error (immediate, not debounced).
	WatcherError WatcherEventType = "error"
)

// WatcherEvent represents an event from the database watcher.
type WatcherEvent struct {
	Type  WatcherEventType
	Error error // Non-nil for WatcherError events
}

// Watcher monitors the beads database for changes and publishes events via broker.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	dbPath    string
	dialect   appbeads.SQLDialect
	debounce  time.Duration
	done      chan struct{}
	broker    *pubsub.Broker[WatcherEvent]
}

// Config holds watcher configuration options.
type Config struct {
	DBPath      string
	DebounceDur time.Duration
	Dialect     appbeads.SQLDialect
}

// DefaultConfig returns sensible defaults for the watcher.
func DefaultConfig(dbPath string, dialect appbeads.SQLDialect) Config {
	debounce := 100 * time.Millisecond
	if dialect == appbeads.DialectMySQL {
		// Dolt server writes trigger multiple rapid journal file updates.
		// A longer debounce coalesces bulk operations (e.g., 5 rapid deletes)
		// into a single refresh, avoiding queries mid-mutation.
		debounce = 500 * time.Millisecond
	}
	return Config{
		DBPath:      dbPath,
		DebounceDur: debounce,
		Dialect:     dialect,
	}
}

// New creates a new database watcher.
func New(cfg Config) (*Watcher, error) {
	log.Debug(log.CatWatcher, "Creating watcher", "dbPath", cfg.DBPath, "debounce", cfg.DebounceDur)
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.ErrorErr(log.CatWatcher, "Failed to create fsnotify watcher", err)
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fsw,
		dbPath:    cfg.DBPath,
		dialect:   cfg.Dialect,
		debounce:  cfg.DebounceDur,
		done:      make(chan struct{}),
		broker:    pubsub.NewBroker[WatcherEvent](),
	}, nil
}

// watchDir returns the directory to watch based on the backend dialect.
// SQLite: parent directory of the .db file (e.g. .beads/)
// Dolt: the noms directory where manifest lives (e.g. .beads/dolt/beads_foo/.dolt/noms)
func (w *Watcher) watchDir() string {
	if w.dialect == appbeads.DialectMySQL {
		return filepath.Join(w.dbPath, ".dolt", "noms")
	}
	return filepath.Dir(w.dbPath)
}

// Start begins watching the database directory.
// Subscribe to watcher events using Broker().Subscribe(ctx) instead of the old channel return.
func (w *Watcher) Start() error {
	dir := w.watchDir()
	if err := w.fsWatcher.Add(dir); err != nil {
		log.ErrorErr(log.CatWatcher, "Failed to watch directory", err, "dir", dir)
		return fmt.Errorf("watching directory %s: %w", dir, err)
	}

	log.Info(log.CatWatcher, "Started watching", "dir", dir)
	go w.loop()

	return nil
}

// Stop terminates the watcher and releases resources.
// CRITICAL SHUTDOWN SEQUENCE: broker.Close() must be called BEFORE fsWatcher.Close().
// This ensures subscribers receive clean channel close notifications before the underlying
// fsnotify watcher is destroyed. Reversing this order could leave subscribers hanging.
func (w *Watcher) Stop() error {
	log.Debug(log.CatWatcher, "Stopping watcher")
	close(w.done)
	w.broker.Close() // Close broker first to notify subscribers
	return w.fsWatcher.Close()
}

// Broker returns the pub/sub broker for subscribing to watcher events.
// The broker is created in New(), so it is always valid even before Start() is called.
func (w *Watcher) Broker() *pubsub.Broker[WatcherEvent] {
	return w.broker
}

// loop processes file system events with debouncing.
func (w *Watcher) loop() {
	var (
		timer   *time.Timer
		pending bool
	)

	for {
		select {
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			// Only react to writes on database files
			if !w.isRelevantEvent(event) {
				continue
			}

			log.Debug(log.CatWatcher, "File event received", "file", event.Name, "op", event.Op.String())

			// Reset or start debounce timer
			if timer == nil {
				log.Debug(log.CatWatcher, "Starting debounce timer", "duration", w.debounce)
				timer = time.NewTimer(w.debounce)
				pending = true
			} else {
				if !timer.Stop() {
					// Drain the timer channel if it already fired
					select {
					case <-timer.C:
					default:
					}
				}
				log.Debug(log.CatWatcher, "Resetting debounce timer", "duration", w.debounce)
				timer.Reset(w.debounce)
				pending = true
			}

		case <-func() <-chan time.Time {
			if timer != nil {
				return timer.C
			}
			return nil
		}():
			if pending {
				log.Debug(log.CatWatcher, "Debounce complete, triggering refresh")
				// Publish DBChanged event to broker (non-blocking by design)
				w.broker.Publish(pubsub.UpdatedEvent, WatcherEvent{
					Type: DBChanged,
				})
				pending = false
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// Log error AND publish error event (immediate, not debounced)
			log.ErrorErr(log.CatWatcher, "File watcher error", err)
			w.broker.Publish(pubsub.UpdatedEvent, WatcherEvent{
				Type:  WatcherError,
				Error: err,
			})

		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// isRelevantEvent checks if the event should trigger a refresh.
// For SQLite, watches beads.db and beads.db-wal files.
// For Dolt, watches the journal file and manifest in the noms directory.
// In embedded mode, Dolt updates the manifest on each write.
// In server mode, Dolt appends to the journal file (manifest only updates on compaction/shutdown).
func (w *Watcher) isRelevantEvent(event fsnotify.Event) bool {
	// Only care about write or create operations
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	base := filepath.Base(event.Name)
	if w.dialect == appbeads.DialectMySQL {
		// Dolt noms files:
		// - "manifest" changes in embedded mode on each write
		// - journal file (32 'v' chars) changes in server mode on each write
		return base == "manifest" || base == "vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"
	}
	// SQLite: database file and WAL
	return base == "beads.db" || base == "beads.db-wal"
}
