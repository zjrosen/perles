// Package watcher provides file system watching with debouncing for the beads database.
package watcher

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors the beads database for changes and sends notifications.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	dbPath    string
	debounce  time.Duration
	onChange  chan struct{}
	done      chan struct{}
}

// Config holds watcher configuration options.
type Config struct {
	DBPath      string
	DebounceDur time.Duration
}

// DefaultConfig returns sensible defaults for the watcher.
func DefaultConfig(dbPath string) Config {
	return Config{
		DBPath:      dbPath,
		DebounceDur: 1 * time.Second,
	}
}

// New creates a new database watcher.
func New(cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fsw,
		dbPath:    cfg.DBPath,
		debounce:  cfg.DebounceDur,
		onChange:  make(chan struct{}, 1),
		done:      make(chan struct{}),
	}, nil
}

// Start begins watching the database directory.
// Returns a channel that receives a signal when the database changes.
func (w *Watcher) Start() (<-chan struct{}, error) {
	// Watch the directory containing the database
	dir := filepath.Dir(w.dbPath)
	if err := w.fsWatcher.Add(dir); err != nil {
		return nil, fmt.Errorf("watching directory %s: %w", dir, err)
	}

	go w.loop()

	return w.onChange, nil
}

// Stop terminates the watcher and releases resources.
func (w *Watcher) Stop() error {
	close(w.done)
	return w.fsWatcher.Close()
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

			// Reset or start debounce timer
			if timer == nil {
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
				// Non-blocking send - drop if channel full
				select {
				case w.onChange <- struct{}{}:
				default:
				}
				pending = false
			}

		case _, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
			// Note: We intentionally don't log here to avoid dependency on a logger.
			// Callers can wrap the watcher if they need error visibility.

		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// isRelevantEvent checks if the event should trigger a refresh.
func (w *Watcher) isRelevantEvent(event fsnotify.Event) bool {
	// Only care about write or create operations (WAL file may be created fresh)
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	base := filepath.Base(event.Name)
	return base == "beads.db" || base == "beads.db-wal"
}
