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
	"github.com/zjrosen/perles/internal/task"
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

// Clock tells a SnapshotBackend the time.
type Clock interface {
	Now() time.Time
}

// systemClock is the Clock of a SnapshotBackend configured without one.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SnapshotBackendConfig configures a SnapshotBackend. BeadsDir, ManifestPath
// and Runner are required.
type SnapshotBackendConfig struct {
	// BeadsDir is the embedded workspace's .beads directory.
	BeadsDir string
	// WorkDir is the directory bd runs in for writes, ShowIssue and the
	// comment fallback.
	WorkDir string
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
	// Runner runs the bd reads that build snapshots (infrabeads.NewBDRunner
	// in production).
	Runner infrabeads.BDRunner
	// Clock times builds; nil means the system clock.
	Clock Clock
}

// SnapshotBackend is the beads backend of an embedded Dolt workspace, which
// has no SQL server to query. It answers queries from a snapshot: an
// in-memory SQLite database (memdb) loaded from bounded --readonly bd reads
// and queried by the BQL executor in its SQLite dialect. Writes, ShowIssue
// and the comment fallback go through the bd CLI.
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
// a snapshot is installed, queries and snapshot comment reads never run bd
// and never wait on an export.
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

	// installMu orders installs with Close: once closed is set, no snapshot
	// is installed and the built one is discarded.
	installMu sync.Mutex
	closed    atomic.Bool

	// firstMu guards firstBuild, the lazy first build in flight.
	firstMu    sync.Mutex
	firstBuild *pendingBuild

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
// by cfg. It runs no bd and builds no snapshot: the first query or
// CheckCompatibility does.
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
		ctx:          ctx,
		cancel:       cancel,
	}

	bdExec := infrabeads.NewBDExecutor(cfg.WorkDir, cfg.BeadsDir)
	b.services = task.BackendServices{
		TaskExecutor: NewBeadsTaskExecutor(
			bdExec,
			WithCommentReader(snapshotCommentReader{b: b}),
			WithFallbackCommentReader(bdExec),
		),
		QueryExecutor:     NewBeadsQueryExecutor(snapshotQueries{b: b}),
		QueryHelpers:      NewBeadsQueryHelpers(),
		SyntaxHighlighter: NewBeadsSyntaxHighlighter(),
		Capabilities:      beadsCapabilities(),
		// No DBPath: the app creates no file watcher. bd rewrites the
		// manifest on every command, reads included, so the backend
		// watches it itself.
		DBPath: "",
	}
	return b, nil
}

// Services returns the task-layer services produced by this backend.
func (b *SnapshotBackend) Services() task.BackendServices {
	return b.services
}

// CheckCompatibility builds and installs the first snapshot when none is
// installed yet, so the export runs before the UI starts, and returns the
// build's error unchanged: the embedded bd version gate's
// *infrabeads.UnsupportedWorkspaceError or bd's translated error.
func (b *SnapshotBackend) CheckCompatibility() error {
	return b.buildFirst()
}

// FlushCaches does nothing: each snapshot is immutable and owns its BQL
// caches, so they cannot be stale for the snapshot they belong to.
func (b *SnapshotBackend) FlushCaches(context.Context) error {
	return nil
}

// Close cancels the backend's running bd reads, retires the current snapshot
// (waiting for its in-flight readers) and deletes its memdb. A build that
// finishes afterwards is discarded. Reads after Close fail. Close is
// idempotent.
func (b *SnapshotBackend) Close() error {
	b.cancel(errSnapshotBackendClosed)

	b.installMu.Lock()
	if b.closed.Swap(true) {
		b.installMu.Unlock()
		return nil
	}
	old := b.current.Swap(nil)
	b.installMu.Unlock()

	if old == nil {
		return nil
	}
	if err := old.retire(); err != nil {
		return fmt.Errorf("closing snapshot database: %w", err)
	}
	return nil
}

// buildFirst builds and installs the first snapshot when none is installed.
// Concurrent callers share one build and its result. A failed build installs
// nothing, so the next caller builds again.
func (b *SnapshotBackend) buildFirst() error {
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

// rebuild builds the next generation and installs it.
func (b *SnapshotBackend) rebuild(ctx context.Context) error {
	next, err := b.build(ctx)
	if err != nil {
		return err
	}
	return b.install(next)
}

// build makes the next snapshot generation without installing it:
//
//  1. the embedded bd version gate, before any bd call;
//  2. the manifest hash and writeSeq, read before exporting, so the
//     snapshot claims no more than the export can cover;
//  3. the bd reads: export, blocked flags, custom statuses;
//  4. only then, a fresh memdb database with a unique per-generation name,
//     and a BQL executor over it with fresh caches.
//
// Every bd read runs before the memdb database is created, so a bd failure
// leaves nothing to free. A failure from the load on deletes the database.
// bd's errors are returned unchanged.
func (b *SnapshotBackend) build(ctx context.Context) (*snapshot, error) {
	start := b.clock.Now()
	if err := infrabeads.CheckEmbeddedBDVersion(b.beadsDir, b.bdVersion, b.bdContextErr); err != nil {
		return nil, err
	}
	manifestHash, err := hashManifest(b.manifestPath)
	if err != nil {
		return nil, err
	}
	coveredWriteSeq := b.writeSeq.Load()

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
// replaces. After Close it discards next instead and returns
// errSnapshotBackendClosed.
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
		if err = b.buildFirst(); err == nil {
			err = b.withCurrent(runQuery)
		}
	}
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// snapshotComments returns an issue's comments from the current snapshot,
// oldest first. It never runs bd, and builds no snapshot: before the first
// one it returns errNoSnapshot.
func (b *SnapshotBackend) snapshotComments(issueID string) ([]beads.Comment, error) {
	var comments []beads.Comment
	err := b.withCurrent(func(s *snapshot) error {
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
// fresh caches, so no cached result outlives its generation.
func newSnapshotExecutor(db *sql.DB) *bql.Executor {
	bqlCache := cachemanager.NewInMemoryCacheManager[string, []beads.Issue](
		"snapshot-bql-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)
	depGraphCache := cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph](
		"snapshot-bql-dep-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
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

// snapshotCommentReader reads comments from a backend's current snapshot.
type snapshotCommentReader struct{ b *SnapshotBackend }

func (r snapshotCommentReader) GetComments(issueID string) ([]beads.Comment, error) {
	return r.b.snapshotComments(issueID)
}
