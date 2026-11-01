package adapter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/go-sqlite3/vfs/memdb"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	"github.com/zjrosen/perles/internal/beads/bql"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/watcher"
)

// Compile-time checks.
var (
	_ task.Backend           = (*SnapshotBackend)(nil)
	_ bql.BQLExecutor        = snapshotQueries{}
	_ appbeads.CommentReader = snapshotCommentReader{}
)

// errSnapshotBackendClosed is returned by reads after Close, and is the cause
// with which Close cancels the backend's running bd reads.
var errSnapshotBackendClosed = errors.New("snapshot backend closed")

// errNoSnapshot reports that no snapshot is installed yet.
var errNoSnapshot = errors.New("no snapshot installed yet")

// snapshotBackendIDs numbers the process's snapshot backends. memdb names are
// process-wide, so each backend's generations get their own name prefix.
var snapshotBackendIDs atomic.Uint64

// Clock tells a SnapshotBackend the time, and when to wake its scheduler.
type Clock interface {
	Now() time.Time
	// WaitUntil returns a channel that receives the time once it is
	// deadline or later (at once when deadline has passed).
	WaitUntil(deadline time.Time) <-chan time.Time
}

// systemClock is the Clock of a SnapshotBackend configured without one.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) WaitUntil(deadline time.Time) <-chan time.Time {
	return time.After(time.Until(deadline))
}

// SnapshotBackendConfig configures a SnapshotBackend. BeadsDir, ManifestPath
// and Runner are required.
type SnapshotBackendConfig struct {
	// BeadsDir is the embedded workspace's .beads directory.
	BeadsDir string
	// ManifestPath is the embedded database's noms manifest
	// (infrabeads.EmbeddedManifestPath). Every build hashes it before
	// exporting.
	ManifestPath string
	// BDVersion and BDContextErr are the workspace's
	// EmbeddedWorkspaceError.BDVersion and BDContextErr. Every build runs the
	// embedded bd version gate on them (infrabeads.CheckEmbeddedBDVersion)
	// before any bd call, so they must be copied from the error: left zero,
	// the gate refuses the workspace because it could not read the bd version.
	BDVersion    string
	BDContextErr error
	// Runner runs every bd command of the backend: the reads that build
	// snapshots, the writes, ShowIssue and the comment reads. In production
	// it is infrabeads.NewBDRunner(workDir, beadsDir), which sets where bd
	// runs.
	Runner infrabeads.BDRunner
	// Clock times builds and paces rebuilds; nil means the system clock.
	Clock Clock
}

// SnapshotBackend is the beads backend of an embedded Dolt workspace, which
// has no SQL server to query. It answers queries from a snapshot: an
// in-memory SQLite database (memdb) loaded from bounded --readonly bd reads
// and queried by the BQL executor in its SQLite dialect. Writes, ShowIssue
// and the comment reads made before the first snapshot go through the bd
// CLI. After perles adds a comment, the issue's comments are served from an
// overlay until a snapshot covering the write is installed (see
// snapshotIssueExecutor).
//
// The installed snapshot is swapped atomically for a newer generation. A
// reader takes the current snapshot's read lock for its whole call and
// retries on the new pointer if it finds the snapshot retired. Retirement
// runs only after the new snapshot is stored: it takes the write lock (so it
// waits for in-flight readers), marks the snapshot retired, then closes its
// database and deletes its memdb. A reader therefore never sees
// "sql: database is closed", nor results mixing two generations.
//
// The first snapshot is built lazily, by the first query (or by
// CheckCompatibility), with concurrent first callers sharing one build. Once
// a snapshot is installed, queries and comment reads never run bd and never
// wait on an export.
//
// Later snapshots are built by the scheduler (snapshot_scheduler.go), when
// the manifest watcher, perles' own writes or FlushCaches find the manifest
// changed, and the backend publishes each install on Changes().
//
// Create it with NewSnapshotBackend; it is safe for concurrent use.
type SnapshotBackend struct {
	id           uint64
	beadsDir     string
	manifestPath string
	bdVersion    string
	bdContextErr error
	run          infrabeads.BDRunner
	clock        Clock
	blocked      *infrabeads.BlockedFlagSource
	// load loads one generation into a fresh memdb database. It is
	// infrabeads.LoadSnapshot; tests wrap it to act between the load and the
	// install.
	load func(ctx context.Context, name string, data infrabeads.SnapshotData) (*sql.DB, error)
	// bdReads runs the bd reads that build no snapshot (ShowIssue, the
	// comments read back after AddComment, and comment reads made before the
	// first snapshot) through run with ctx as their parent, so Close cancels
	// them and later ones fail.
	bdReads *infrabeads.BDExecutor

	// ctx is the parent context of every bd read; Close cancels it with
	// errSnapshotBackendClosed.
	ctx    context.Context
	cancel context.CancelCauseFunc

	// current is the installed snapshot, nil before the first install and
	// after Close.
	current atomic.Pointer[snapshot]
	// writeSeq counts the successful writes made through perles. A build
	// records it before exporting as the snapshot's coveredWriteSeq.
	writeSeq atomic.Uint64
	// lastGen is the last generation number handed to a build.
	lastGen atomic.Uint64

	// overlayMu guards overlay. It is taken after a snapshot's read lock,
	// never before one.
	overlayMu sync.Mutex
	// overlay holds, per issue, the comments read straight after perles
	// added one. GetComments serves an entry while the snapshot it reads does
	// not cover the entry's write; install drops the entries a new snapshot
	// covers.
	overlay map[string]commentOverlay

	// installMu orders installs with Close: once closed is set, no snapshot
	// is installed and the built one is discarded.
	installMu sync.Mutex
	closed    atomic.Bool

	// firstMu guards firstBuild, the lazy first build in flight.
	firstMu    sync.Mutex
	firstBuild *pendingBuild

	// sched decides when to rebuild, and serializes builds.
	sched scheduler
	// changes is the Changes() broker: created here, closed by Close.
	changes *pubsub.Broker[watcher.WatcherEvent]

	services task.BackendServices
}

// snapshot is one installed generation: its memdb database, the BQL
// executor over it with its own caches, and what the build covered.
type snapshot struct {
	db        *sql.DB
	exec      *bql.Executor
	memdbName string
	// manifestHash is the manifest's hash read before the export.
	manifestHash string
	// coveredWriteSeq is writeSeq read before the export: the snapshot
	// includes every perles write up to it.
	coveredWriteSeq uint64
	gen             uint64

	// mu is held for reading by every reader for its whole call, and for
	// writing by retire to set retired.
	mu      sync.RWMutex
	retired bool
}

// pendingBuild is a lazy first build that concurrent first callers wait on.
type pendingBuild struct {
	done chan struct{}
	err  error // set before done is closed
}

// NewSnapshotBackend returns the backend of the embedded workspace described
// by cfg. It runs no bd, builds no snapshot and starts no goroutine: the
// first query or CheckCompatibility builds the first snapshot, and its
// install starts the scheduler and the manifest watcher.
func NewSnapshotBackend(cfg SnapshotBackendConfig) (*SnapshotBackend, error) {
	switch {
	case cfg.BeadsDir == "":
		return nil, errors.New("snapshot backend: no beads directory")
	case cfg.ManifestPath == "":
		return nil, errors.New("snapshot backend: no embedded Dolt manifest path")
	case cfg.Runner == nil:
		return nil, errors.New("snapshot backend: no bd runner")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	b := &SnapshotBackend{
		id:           snapshotBackendIDs.Add(1),
		beadsDir:     cfg.BeadsDir,
		manifestPath: cfg.ManifestPath,
		bdVersion:    cfg.BDVersion,
		bdContextErr: cfg.BDContextErr,
		run:          cfg.Runner,
		clock:        clock,
		blocked:      infrabeads.NewBlockedFlagSource(cfg.Runner),
		load:         infrabeads.LoadSnapshot,
		bdReads:      infrabeads.NewBDExecutorWithRunner(ctx, cfg.Runner),
		ctx:          ctx,
		cancel:       cancel,
		overlay:      map[string]commentOverlay{},
		changes:      pubsub.NewBroker[watcher.WatcherEvent](),
	}
	b.sched.wake = make(chan struct{}, 1)
	b.sched.newWatcher = b.watchManifest

	// No WithFallbackCommentReader: BeadsTaskExecutor falls back on any
	// primary-reader error, which would run bd on the caller's goroutine
	// when an installed snapshot fails to read. The comment reader runs bd
	// only while no snapshot exists yet.
	b.services = task.BackendServices{
		TaskExecutor: NewBeadsTaskExecutor(
			snapshotIssueExecutor{
				b: b,
				// Writes do not run with b.ctx: Close must not kill a
				// write in flight, which could lose it. bd's timeout
				// bounds them, as in server mode.
				writes: infrabeads.NewBDExecutorWithRunner(context.Background(), cfg.Runner),
			},
			WithCommentReader(snapshotCommentReader{b: b}),
		),
		QueryExecutor:     NewBeadsQueryExecutor(snapshotQueries{b: b}),
		QueryHelpers:      NewBeadsQueryHelpers(),
		SyntaxHighlighter: NewBeadsSyntaxHighlighter(),
		Capabilities:      beadsCapabilities(),
		// No DBPath: the app creates no file watcher and listens to
		// Changes() instead. bd rewrites the manifest on every command,
		// reads included, so the backend watches it itself and publishes
		// installs only.
		DBPath: "",
	}
	return b, nil
}

// Services returns the task-layer services produced by this backend.
func (b *SnapshotBackend) Services() task.BackendServices {
	return b.services
}

// CheckCompatibility builds and installs the first snapshot when none is
// installed yet, so the export runs before the UI starts. Unlike a query, it
// builds even while the failure backoff defers retries.
//
// A failed build is reported as a *task.UnsupportedWorkspaceError of kind
// WorkspaceEmbeddedDolt, so the startup screen explains it: its Detail names
// the cause (bd-version for the embedded bd version gate, bd-not-found,
// bd-error for bd's translated error, workspace otherwise; see
// embeddedStartupError), and it wraps the build's error.
func (b *SnapshotBackend) CheckCompatibility() error {
	if err := b.buildFirst(true); err != nil {
		return embeddedStartupError(b.beadsDir, err)
	}
	return nil
}

// Close cancels the backend's running bd reads (a running build fails and
// is discarded), stops the manifest watcher and the scheduler goroutine,
// closes the Changes broker, and retires the current snapshot (waiting for
// its in-flight readers) and deletes its memdb. A build that finishes
// afterwards is discarded. Reads after Close fail, ShowIssue included.
// Writes do not depend on the snapshot: Close neither cancels one in flight
// nor refuses later ones. Close is idempotent.
func (b *SnapshotBackend) Close() error {
	b.cancel(errSnapshotBackendClosed)

	b.installMu.Lock()
	if b.closed.Swap(true) {
		b.installMu.Unlock()
		return nil
	}
	old := b.current.Swap(nil)
	b.installMu.Unlock()

	err := b.stopLoops()
	b.changes.Close()
	if old != nil {
		if retireErr := old.retire(); retireErr != nil {
			err = errors.Join(err, fmt.Errorf("closing snapshot database: %w", retireErr))
		}
	}
	return err
}

// buildFirst builds and installs the first snapshot when none is installed.
// Concurrent callers share one build and its result. A failed build installs
// nothing, so a later caller builds again, but a query (explicit false) does
// so only once the scheduler's pacing and failure backoff allow; until then
// it returns the last build's error at once without running bd. A broken
// workspace therefore spawns no bd per query, in the daemon too (it never
// calls CheckCompatibility). FlushCaches resets the backoff. An explicit
// request (CheckCompatibility) always builds.
func (b *SnapshotBackend) buildFirst(explicit bool) error {
	b.firstMu.Lock()
	if b.current.Load() != nil {
		b.firstMu.Unlock()
		return nil
	}
	if p := b.firstBuild; p != nil {
		b.firstMu.Unlock()
		<-p.done
		return p.err
	}
	if b.closed.Load() {
		b.firstMu.Unlock()
		return errSnapshotBackendClosed
	}
	if !explicit {
		if err := b.deferredBuildErr(); err != nil {
			b.firstMu.Unlock()
			return err
		}
	}
	p := &pendingBuild{done: make(chan struct{})}
	b.firstBuild = p
	b.firstMu.Unlock()

	defer func() {
		b.firstMu.Lock()
		b.firstBuild = nil
		b.firstMu.Unlock()
		close(p.done)
	}()
	p.err = b.rebuild(b.ctx)
	return p.err
}

// build makes the next snapshot generation without installing it:
//
//  1. the embedded bd version gate, before any bd call;
//  2. the manifest hash and writeSeq, read before exporting, so the
//     snapshot claims no more than the export can cover. started is then
//     called with the hash, still before the export (rebuild publishes it
//     as the in-flight hash);
//  3. the bd reads: export, blocked flags, custom statuses;
//  4. only then, a fresh memdb database with a unique per-generation name,
//     and a BQL executor over it with fresh caches.
//
// Every bd read runs before the memdb database is created, so a bd failure
// leaves nothing to free. A failure from the load on deletes the database.
// bd's errors are returned unchanged.
func (b *SnapshotBackend) build(ctx context.Context, started func(manifestHash string)) (*snapshot, error) {
	start := b.clock.Now()
	if err := infrabeads.CheckEmbeddedBDVersion(b.beadsDir, b.bdVersion, b.bdContextErr); err != nil {
		return nil, err
	}
	manifestHash, err := hashManifest(b.manifestPath)
	if err != nil {
		return nil, err
	}
	coveredWriteSeq := b.writeSeq.Load()
	started(manifestHash)

	data, err := b.readSnapshotData(ctx)
	if err != nil {
		return nil, err
	}

	gen := b.lastGen.Add(1)
	name := snapshotMemdbName(b.id, gen)
	db, err := b.load(ctx, name, data)
	if err != nil {
		if db != nil {
			if closeErr := db.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("closing snapshot database: %w", closeErr))
			}
		}
		memdb.Delete(name)
		return nil, fmt.Errorf("loading snapshot: %w", err)
	}

	s := &snapshot{
		db:              db,
		exec:            newSnapshotExecutor(db),
		memdbName:       name,
		manifestHash:    manifestHash,
		coveredWriteSeq: coveredWriteSeq,
		gen:             gen,
	}
	log.Debug(log.CatBeads, "Built snapshot", "name", name, "issues", len(data.Issues),
		"coveredWriteSeq", coveredWriteSeq, "duration", b.clock.Now().Sub(start))
	return s, nil
}

// readSnapshotData runs the bd reads of one build: the export, then the
// blocked flags and the custom statuses it needs.
func (b *SnapshotBackend) readSnapshotData(ctx context.Context) (infrabeads.SnapshotData, error) {
	out, err := infrabeads.ReadSnapshotExport(ctx, b.run)
	if err != nil {
		return infrabeads.SnapshotData{}, err
	}
	issues, err := infrabeads.DecodeExport(strings.NewReader(out))
	if err != nil {
		return infrabeads.SnapshotData{}, err
	}
	blocked, err := b.blocked.ReadBlockedIDs(ctx, issues)
	if err != nil {
		return infrabeads.SnapshotData{}, err
	}
	statuses, err := infrabeads.ReadCustomStatuses(ctx, b.run, issues)
	if err != nil {
		return infrabeads.SnapshotData{}, err
	}
	return infrabeads.SnapshotData{
		Issues:         issues,
		BlockedIDs:     blocked,
		CustomStatuses: statuses,
		BDVersion:      b.bdVersion,
	}, nil
}

// install makes next the current snapshot, then retires the one it
// replaces and drops the comment overlay entries next covers. After Close it
// discards next instead and returns errSnapshotBackendClosed. Its callers
// hold the scheduler's buildMu, so installs run one at a time.
func (b *SnapshotBackend) install(next *snapshot) error {
	b.installMu.Lock()
	if b.closed.Load() {
		b.installMu.Unlock()
		if err := next.retire(); err != nil {
			log.Warn(log.CatBeads, "Closing discarded snapshot failed", "name", next.memdbName, "error", err)
		}
		return errSnapshotBackendClosed
	}
	old := b.current.Swap(next)
	b.installMu.Unlock()

	if old != nil {
		if err := old.retire(); err != nil {
			log.Warn(log.CatBeads, "Closing retired snapshot failed", "name", old.memdbName, "error", err)
		}
	}
	// Prune only after the retire: until it returned, a reader could still
	// hold old, which lacks the overlaid comments, and need the entries.
	b.pruneOverlay(next.coveredWriteSeq)
	return nil
}

// withCurrent runs use on the current snapshot under its read lock, which it
// holds for the whole call. It returns errNoSnapshot when none is installed
// yet, and errSnapshotBackendClosed after Close. The snapshot is acquired
// exactly once per call: use must not acquire one again, because a
// recursive read lock can deadlock behind a pending retire.
func (b *SnapshotBackend) withCurrent(use func(*snapshot) error) error {
	for {
		s := b.current.Load()
		if s == nil {
			if b.closed.Load() {
				return errSnapshotBackendClosed
			}
			return errNoSnapshot
		}
		s.mu.RLock()
		if !s.retired {
			defer s.mu.RUnlock()
			return use(s)
		}
		// Replaced and retired between Load and RLock. current was stored
		// before the retire, so the retry sees a newer snapshot (or nil
		// after Close).
		s.mu.RUnlock()
	}
}

// execute runs a BQL query on the current snapshot. When no snapshot is
// installed yet it builds the first one first; otherwise it reads memory
// only, never running bd.
func (b *SnapshotBackend) execute(query string) ([]beads.Issue, error) {
	var issues []beads.Issue
	runQuery := func(s *snapshot) error {
		var err error
		issues, err = s.exec.Execute(query)
		return err
	}
	err := b.withCurrent(runQuery)
	if errors.Is(err, errNoSnapshot) {
		if err = b.buildFirst(false); err == nil {
			err = b.withCurrent(runQuery)
		}
	}
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// snapshotComments returns an issue's comments as of the current snapshot,
// oldest first: its overlay entry when the snapshot does not cover the
// entry's write, the snapshot's comments otherwise. It reads memory only,
// never running bd, and builds no snapshot: before the first one it returns
// errNoSnapshot.
func (b *SnapshotBackend) snapshotComments(issueID string) ([]beads.Comment, error) {
	var comments []beads.Comment
	err := b.withCurrent(func(s *snapshot) error {
		if overlaid, ok := b.liveOverlay(s, issueID); ok {
			comments = overlaid
			return nil
		}
		var err error
		comments, err = s.comments(issueID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// comments reads an issue's comments, oldest first. Snapshot times have
// whole seconds, so comments made in the same second keep bd's export order
// (rowid).
func (s *snapshot) comments(issueID string) ([]beads.Comment, error) {
	rows, err := s.db.QueryContext(context.Background(), `
		SELECT id, author, text, created_at
		FROM comments
		WHERE issue_id = ?
		ORDER BY created_at ASC, rowid ASC
	`, issueID)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot comments of %s: %w", issueID, err)
	}
	defer func() { _ = rows.Close() }()

	var comments []beads.Comment
	for rows.Next() {
		var c beads.Comment
		if err := rows.Scan(&c.ID, &c.Author, &c.Text, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("reading snapshot comments of %s: %w", issueID, err)
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading snapshot comments of %s: %w", issueID, err)
	}
	return comments, nil
}

// retire marks the snapshot retired once its in-flight readers are done,
// then closes its database and deletes its memdb. Readers that acquire it
// later see retired and retry on the current snapshot.
func (s *snapshot) retire() error {
	s.mu.Lock()
	s.retired = true
	s.mu.Unlock()
	err := s.db.Close()
	memdb.Delete(s.memdbName)
	return err
}

// newSnapshotExecutor returns a BQL executor over a snapshot database with
// fresh caches, so no cached result outlives its generation. The caches run
// no janitor (cleanup interval 0): a janitor is a goroutine per cache, freed
// only by a finalizer, which would pile up across generations. Expired
// entries are still never returned, and each cache goes with its snapshot.
func newSnapshotExecutor(db *sql.DB) *bql.Executor {
	bqlCache := cachemanager.NewInMemoryCacheManager[string, []beads.Issue](
		"snapshot-bql-cache",
		cachemanager.DefaultExpiration,
		0,
	)
	depGraphCache := cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph](
		"snapshot-bql-dep-cache",
		cachemanager.DefaultExpiration,
		0,
	)
	return bql.NewExecutor(db, appbeads.DialectSQLite, bqlCache, depGraphCache)
}

// snapshotMemdbName is the memdb name of a backend's generation. It has no
// leading slash (infrabeads.LoadSnapshot adds it to the DSN).
func snapshotMemdbName(backendID, gen uint64) string {
	return fmt.Sprintf("perles-snap-%d-%d", backendID, gen)
}

// hashManifest returns the hex SHA-256 of the manifest file. bd rewrites
// the manifest on every command, with the same bytes unless the database
// changed.
func hashManifest(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the workspace's own manifest path
	if err != nil {
		return "", fmt.Errorf("reading embedded Dolt manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// snapshotQueries is the BQL executor over a backend's current snapshot.
type snapshotQueries struct{ b *SnapshotBackend }

func (q snapshotQueries) Execute(query string) ([]beads.Issue, error) {
	return q.b.execute(query)
}

// snapshotCommentReader is the comment reader of a backend's TaskExecutor
// (see SnapshotBackend.getComments).
type snapshotCommentReader struct{ b *SnapshotBackend }

func (r snapshotCommentReader) GetComments(issueID string) ([]beads.Comment, error) {
	return r.b.getComments(issueID)
}
