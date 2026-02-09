package migrations

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/stretchr/testify/require"

	// Import ncruces driver - this is the same driver Perles uses
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TestRunMigrations_FreshDB verifies all migrations apply to an empty :memory: database.
func TestRunMigrations_FreshDB(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err, "ncruces driver should open :memory: database")
	defer db.Close()

	// Run migrations
	err = RunMigrations(db)
	require.NoError(t, err, "RunMigrations should succeed on fresh database")

	// Verify sessions table was created
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&tableName)
	require.NoError(t, err, "sessions table should exist")
	require.Equal(t, "sessions", tableName)
}

// TestRunMigrations_Idempotent verifies calling RunMigrations twice doesn't error.
func TestRunMigrations_Idempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	// First run
	err = RunMigrations(db)
	require.NoError(t, err, "first migration run should succeed")

	// Second run should not error (ErrNoChange handled internally)
	err = RunMigrations(db)
	require.NoError(t, err, "second migration run should not error")

	// Verify table still exists
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&tableName)
	require.NoError(t, err)
	require.Equal(t, "sessions", tableName)
}

// TestMigrations_Schema verifies sessions table exists with correct columns and indexes.
func TestMigrations_Schema(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	err = RunMigrations(db)
	require.NoError(t, err)

	// Verify table has expected columns
	rows, err := db.Query(`PRAGMA table_info(sessions)`)
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt interface{}
		err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk)
		require.NoError(t, err)
		columns[name] = true
	}
	require.NoError(t, rows.Err())

	expectedColumns := []string{"id", "guid", "project", "state", "created_at", "updated_at", "deleted_at"}
	for _, col := range expectedColumns {
		require.True(t, columns[col], "column %s should exist", col)
	}

	// Verify indexes were created
	indexRows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='sessions'`)
	require.NoError(t, err)
	defer indexRows.Close()

	indexes := make(map[string]bool)
	for indexRows.Next() {
		var name string
		require.NoError(t, indexRows.Scan(&name))
		indexes[name] = true
	}
	require.NoError(t, indexRows.Err())

	expectedIndexes := []string{
		"idx_sessions_project",
		"idx_sessions_guid",
		"idx_sessions_deleted_at",
		"idx_sessions_project_state",
	}
	for _, idx := range expectedIndexes {
		require.True(t, indexes[idx], "index %s should exist", idx)
	}
}

// TestMigrations_Down verifies down migration rolls back schema correctly.
func TestMigrations_Down(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	// Apply migrations first using the lower-level API for down testing
	driver, err := WithInstance(db, &Config{})
	require.NoError(t, err)

	source, err := iofs.New(MigrationsFS(), ".")
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	require.NoError(t, err)

	err = m.Up()
	require.NoError(t, err, "migrations should apply")

	// Verify table exists before down
	var tableExists bool
	err = db.QueryRow(`SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&tableExists)
	require.NoError(t, err)
	require.True(t, tableExists, "sessions table should exist before down migration")

	// Run down migrations
	err = m.Down()
	require.NoError(t, err, "down migrations should succeed")

	// Verify table no longer exists
	err = db.QueryRow(`SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&tableExists)
	require.NoError(t, err)
	require.False(t, tableExists, "sessions table should be dropped after down migration")

	// Verify indexes are gone too
	var indexCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='sessions'`).Scan(&indexCount)
	require.NoError(t, err)
	require.Equal(t, 0, indexCount, "all indexes should be dropped")
}

// TestMigrationsFS_Embedded verifies SQL files load from embedded FS at build time.
func TestMigrationsFS_Embedded(t *testing.T) {
	fs := MigrationsFS()
	require.NotNil(t, fs, "MigrationsFS should return non-nil filesystem")

	// Verify we can read directory
	entries, err := embeddedMigrationsFS.ReadDir(".")
	require.NoError(t, err, "should read embedded directory")

	fileNames := make(map[string]bool)
	for _, entry := range entries {
		fileNames[entry.Name()] = true
	}

	require.True(t, fileNames["000001_create_sessions.up.sql"], "up migration should be embedded")
	require.True(t, fileNames["000001_create_sessions.down.sql"], "down migration should be embedded")

	// Read content to verify it's not empty
	upContent, err := embeddedMigrationsFS.ReadFile("000001_create_sessions.up.sql")
	require.NoError(t, err)
	require.Contains(t, string(upContent), "CREATE TABLE sessions")

	downContent, err := embeddedMigrationsFS.ReadFile("000001_create_sessions.down.sql")
	require.NoError(t, err)
	require.Contains(t, string(downContent), "DROP TABLE")
}

// TestNCrucesDriverWithGolangMigrate validates that our custom NCrucesSqlite driver
// works with golang-migrate's migration framework using ncruces/go-sqlite3.
func TestNCrucesDriverWithGolangMigrate(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err, "ncruces driver should open :memory: database")
	defer db.Close()

	// Verify connection works
	err = db.Ping()
	require.NoError(t, err, "database should respond to ping")

	// Create our custom ncruces-compatible driver
	driver, err := WithInstance(db, &Config{})
	require.NoError(t, err, "WithInstance should accept ncruces *sql.DB")
	require.NotNil(t, driver, "driver should not be nil")
}

// TestMigrateUp verifies that embedded migrations run successfully using lower-level API.
func TestMigrateUp(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	driver, err := WithInstance(db, &Config{})
	require.NoError(t, err)

	source, err := iofs.New(MigrationsFS(), ".")
	require.NoError(t, err, "iofs should load embedded SQL files")

	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	require.NoError(t, err, "migrate instance should be created")

	err = m.Up()
	require.NoError(t, err, "migrations should apply successfully")

	// Verify sessions table was created
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&tableName)
	require.NoError(t, err, "sessions table should exist")
	require.Equal(t, "sessions", tableName)
}

// TestMigrateIdempotent verifies that running migrations twice handles ErrNoChange.
func TestMigrateIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	// First migration run
	driver1, err := WithInstance(db, &Config{})
	require.NoError(t, err)

	source1, err := iofs.New(MigrationsFS(), ".")
	require.NoError(t, err)

	m1, err := migrate.NewWithInstance("iofs", source1, "sqlite3", driver1)
	require.NoError(t, err)

	err = m1.Up()
	require.NoError(t, err, "first migration run should succeed")

	// Close and recreate migrator (simulates app restart)
	driver2, err := WithInstance(db, &Config{})
	require.NoError(t, err)

	source2, err := iofs.New(MigrationsFS(), ".")
	require.NoError(t, err)

	m2, err := migrate.NewWithInstance("iofs", source2, "sqlite3", driver2)
	require.NoError(t, err)

	// Second migration run should return ErrNoChange
	err = m2.Up()
	if err != nil {
		require.True(t, errors.Is(err, migrate.ErrNoChange),
			"second migration run should return ErrNoChange, got: %v", err)
	}
}

// TestInsertAndQueryWithMigration verifies the migrated schema works for CRUD.
func TestInsertAndQueryWithMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	err = RunMigrations(db)
	require.NoError(t, err)

	// Insert a test session
	result, err := db.Exec(`
		INSERT INTO sessions (guid, project, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "test-guid-123", "my-project", "running", 1706000000, 1706000000)
	require.NoError(t, err, "insert should succeed")

	id, err := result.LastInsertId()
	require.NoError(t, err)
	require.Equal(t, int64(1), id, "first insert should have ID 1")

	// Query back
	var guid, project, state string
	var createdAt, updatedAt int64
	var deletedAt *int64
	err = db.QueryRow(`
		SELECT guid, project, state, created_at, updated_at, deleted_at
		FROM sessions WHERE id = ?
	`, id).Scan(&guid, &project, &state, &createdAt, &updatedAt, &deletedAt)
	require.NoError(t, err)
	require.Equal(t, "test-guid-123", guid)
	require.Equal(t, "my-project", project)
	require.Equal(t, "running", state)
	require.Nil(t, deletedAt)

	// Test state CHECK constraint
	_, err = db.Exec(`
		INSERT INTO sessions (guid, project, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "test-guid-456", "my-project", "invalid_state", 1706000000, 1706000000)
	require.Error(t, err, "CHECK constraint should reject invalid state")
}

// --- Fabric Graph Migration Tests (000003) ---

// setupFabricDB creates a fresh in-memory DB with all migrations applied and FK enforcement on.
func setupFabricDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)
	err = RunMigrations(db)
	require.NoError(t, err)
	return db
}

// insertThread is a test helper to insert a fabric_threads row with required fields.
func insertThread(t *testing.T, db *sql.DB, sessionID, id, typ, createdBy string, seq int64) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, id, typ, 1706000000, createdBy, seq)
	require.NoError(t, err)
}

// TestFabricGraph_CleanApply verifies migration 000003 applies cleanly and both tables exist.
func TestFabricGraph_CleanApply(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Verify fabric_threads table exists
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='fabric_threads'`).Scan(&name)
	require.NoError(t, err, "fabric_threads table should exist")
	require.Equal(t, "fabric_threads", name)

	// Verify fabric_dependencies table exists
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='fabric_dependencies'`).Scan(&name)
	require.NoError(t, err, "fabric_dependencies table should exist")
	require.Equal(t, "fabric_dependencies", name)
}

// TestFabricGraph_ThreadsSchema verifies fabric_threads has all expected columns and indexes.
func TestFabricGraph_ThreadsSchema(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Verify columns
	rows, err := db.Query(`PRAGMA table_info(fabric_threads)`)
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var colName, typ string
		var notnull, pk int
		var dflt interface{}
		require.NoError(t, rows.Scan(&cid, &colName, &typ, &notnull, &dflt, &pk))
		columns[colName] = true
	}
	require.NoError(t, rows.Err())

	expectedColumns := []string{
		"session_id", "id", "type", "created_at", "created_by",
		"content", "kind", "slug", "title", "purpose",
		"name", "media_type", "size_bytes", "storage_uri", "sha256",
		"mentions_json", "participants_json", "meta_json",
		"seq", "archived_at",
	}
	for _, col := range expectedColumns {
		require.True(t, columns[col], "fabric_threads column %s should exist", col)
	}

	// Verify indexes
	indexRows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='fabric_threads'`)
	require.NoError(t, err)
	defer indexRows.Close()

	indexes := make(map[string]bool)
	for indexRows.Next() {
		var idxName string
		require.NoError(t, indexRows.Scan(&idxName))
		indexes[idxName] = true
	}
	require.NoError(t, indexRows.Err())

	expectedIndexes := []string{
		"idx_fabric_threads_session_slug",
		"idx_fabric_threads_session_seq",
	}
	for _, idx := range expectedIndexes {
		require.True(t, indexes[idx], "fabric_threads index %s should exist", idx)
	}
}

// TestFabricGraph_DependenciesSchema verifies fabric_dependencies has all expected columns and indexes.
func TestFabricGraph_DependenciesSchema(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Verify columns
	rows, err := db.Query(`PRAGMA table_info(fabric_dependencies)`)
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var colName, typ string
		var notnull, pk int
		var dflt interface{}
		require.NoError(t, rows.Scan(&cid, &colName, &typ, &notnull, &dflt, &pk))
		columns[colName] = true
	}
	require.NoError(t, rows.Err())

	expectedColumns := []string{
		"session_id", "thread_id", "depends_on_id", "relation", "created_at",
	}
	for _, col := range expectedColumns {
		require.True(t, columns[col], "fabric_dependencies column %s should exist", col)
	}

	// Verify indexes
	indexRows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='fabric_dependencies'`)
	require.NoError(t, err)
	defer indexRows.Close()

	indexes := make(map[string]bool)
	for indexRows.Next() {
		var idxName string
		require.NoError(t, indexRows.Scan(&idxName))
		indexes[idxName] = true
	}
	require.NoError(t, indexRows.Err())

	expectedIndexes := []string{
		"idx_fabric_deps_thread",
		"idx_fabric_deps_depends_on",
		"idx_fabric_deps_depends_on_relation",
		"idx_fabric_deps_thread_relation",
	}
	for _, idx := range expectedIndexes {
		require.True(t, indexes[idx], "fabric_dependencies index %s should exist", idx)
	}
}

// TestFabricGraph_Idempotent verifies running migrations twice doesn't error.
func TestFabricGraph_Idempotent(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Second run should not error
	err := RunMigrations(db)
	require.NoError(t, err, "second migration run should not error")

	// Verify tables still exist
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('fabric_threads', 'fabric_dependencies')`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count, "both fabric tables should still exist")
}

// TestFabricGraph_Down verifies down migration drops fabric tables cleanly.
func TestFabricGraph_Down(t *testing.T) {
	db, err := sql.Open("sqlite3", "file::memory:")
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	// Apply all migrations
	driver, err := WithInstance(db, &Config{})
	require.NoError(t, err)

	source, err := iofs.New(MigrationsFS(), ".")
	require.NoError(t, err)

	m, err := migrate.NewWithInstance("iofs", source, "sqlite3", driver)
	require.NoError(t, err)

	err = m.Up()
	require.NoError(t, err)

	// Verify fabric tables exist
	var tableCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('fabric_threads', 'fabric_dependencies')`).Scan(&tableCount)
	require.NoError(t, err)
	require.Equal(t, 2, tableCount, "both fabric tables should exist before down")

	// Step down just migration 000003 (from version 3 to 2)
	err = m.Steps(-1)
	require.NoError(t, err, "stepping down one migration should succeed")

	// Verify fabric tables are dropped
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('fabric_threads', 'fabric_dependencies')`).Scan(&tableCount)
	require.NoError(t, err)
	require.Equal(t, 0, tableCount, "fabric tables should be dropped after down migration")

	// Verify fabric indexes are gone
	var indexCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name LIKE 'idx_fabric_%'`).Scan(&indexCount)
	require.NoError(t, err)
	require.Equal(t, 0, indexCount, "all fabric indexes should be dropped")

	// Verify sessions table still exists (not affected by fabric down migration)
	var sessionsExists bool
	err = db.QueryRow(`SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='sessions'`).Scan(&sessionsExists)
	require.NoError(t, err)
	require.True(t, sessionsExists, "sessions table should still exist after fabric down migration")
}

// TestFabricGraph_UniqueSlugConstraint verifies unique partial index on (session_id, slug).
func TestFabricGraph_UniqueSlugConstraint(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Insert a channel with a slug
	_, err := db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, slug, seq)
		VALUES ('sess-1', 'ch-1', 'channel', 1706000000, 'agent-1', 'general', 1)
	`)
	require.NoError(t, err)

	// Insert another channel with the SAME slug in the SAME session should fail
	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, slug, seq)
		VALUES ('sess-1', 'ch-2', 'channel', 1706000000, 'agent-1', 'general', 2)
	`)
	require.Error(t, err, "duplicate slug in same session should violate unique constraint")

	// Insert same slug in a DIFFERENT session should succeed
	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, slug, seq)
		VALUES ('sess-2', 'ch-3', 'channel', 1706000000, 'agent-1', 'general', 1)
	`)
	require.NoError(t, err, "same slug in different session should be allowed")

	// Insert multiple NULL slugs in the same session should succeed (partial index)
	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
		VALUES ('sess-1', 'msg-1', 'message', 1706000000, 'agent-1', 3)
	`)
	require.NoError(t, err, "first NULL slug should succeed")

	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
		VALUES ('sess-1', 'msg-2', 'message', 1706000000, 'agent-1', 4)
	`)
	require.NoError(t, err, "second NULL slug should succeed (partial index allows multiple NULLs)")
}

// TestFabricGraph_ForeignKeyConstraint verifies FK enforcement on fabric_dependencies.
func TestFabricGraph_ForeignKeyConstraint(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Insert a valid thread
	insertThread(t, db, "sess-1", "thread-1", "message", "agent-1", 1)

	// Try to insert a dependency referencing a non-existent thread_id
	_, err := db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'nonexistent', 'thread-1', 'child_of', 1706000000)
	`)
	require.Error(t, err, "FK should reject dependency with non-existent thread_id")

	// Try to insert a dependency referencing a non-existent depends_on_id
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'thread-1', 'nonexistent', 'child_of', 1706000000)
	`)
	require.Error(t, err, "FK should reject dependency with non-existent depends_on_id")

	// Insert second thread and a valid dependency
	insertThread(t, db, "sess-1", "thread-2", "channel", "agent-1", 2)
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'thread-1', 'thread-2', 'child_of', 1706000000)
	`)
	require.NoError(t, err, "valid dependency should insert successfully")
}

// TestFabricGraph_CompositePrimaryKey verifies the composite PK enforces idempotency.
func TestFabricGraph_CompositePrimaryKey(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Insert two threads
	insertThread(t, db, "sess-1", "thread-1", "message", "agent-1", 1)
	insertThread(t, db, "sess-1", "thread-2", "channel", "agent-1", 2)

	// Insert a dependency
	_, err := db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'thread-1', 'thread-2', 'child_of', 1706000000)
	`)
	require.NoError(t, err)

	// Insert the EXACT same dependency again should fail (composite PK)
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'thread-1', 'thread-2', 'child_of', 1706000001)
	`)
	require.Error(t, err, "duplicate composite key should violate primary key constraint")

	// Same thread_id and depends_on_id but DIFFERENT relation should succeed
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'thread-1', 'thread-2', 'reply_to', 1706000000)
	`)
	require.NoError(t, err, "different relation should be allowed (different composite key)")

	// Same edge in a DIFFERENT session should succeed
	insertThread(t, db, "sess-2", "thread-1", "message", "agent-1", 1)
	insertThread(t, db, "sess-2", "thread-2", "channel", "agent-1", 2)
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-2', 'thread-1', 'thread-2', 'child_of', 1706000000)
	`)
	require.NoError(t, err, "same edge in different session should be allowed")
}

// TestFabricGraph_CrossSessionIsolation verifies same thread ID in different sessions doesn't collide.
func TestFabricGraph_CrossSessionIsolation(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Insert same thread ID in two different sessions
	_, err := db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, content, seq)
		VALUES ('sess-1', 'msg-abc', 'message', 1706000000, 'agent-1', 'Hello from session 1', 1)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, content, seq)
		VALUES ('sess-2', 'msg-abc', 'message', 1706000000, 'agent-2', 'Hello from session 2', 1)
	`)
	require.NoError(t, err, "same thread ID in different session should not collide")

	// Verify each session has its own data
	var content string
	err = db.QueryRow(`SELECT content FROM fabric_threads WHERE session_id = 'sess-1' AND id = 'msg-abc'`).Scan(&content)
	require.NoError(t, err)
	require.Equal(t, "Hello from session 1", content)

	err = db.QueryRow(`SELECT content FROM fabric_threads WHERE session_id = 'sess-2' AND id = 'msg-abc'`).Scan(&content)
	require.NoError(t, err)
	require.Equal(t, "Hello from session 2", content)

	// Verify session-scoped count
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM fabric_threads WHERE session_id = 'sess-1'`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "session 1 should only see its own thread")
}

// TestFabricGraph_TypeCheckConstraint verifies CHECK constraints on type and relation columns.
func TestFabricGraph_TypeCheckConstraint(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Valid thread types should work
	for _, validType := range []string{"channel", "message", "artifact"} {
		_, err := db.Exec(`
			INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
			VALUES ('sess-1', ?, ?, 1706000000, 'agent-1', ?)
		`, "id-"+validType, validType, 1)
		require.NoError(t, err, "valid type %q should be accepted", validType)
		// Clean up to avoid seq conflicts
		_, _ = db.Exec(`DELETE FROM fabric_threads WHERE session_id = 'sess-1' AND id = ?`, "id-"+validType)
	}

	// Invalid thread type should fail
	_, err := db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
		VALUES ('sess-1', 'bad-type', 'invalid_type', 1706000000, 'agent-1', 99)
	`)
	require.Error(t, err, "CHECK constraint should reject invalid thread type")

	// Valid relation types should work
	insertThread(t, db, "sess-1", "t1", "message", "agent-1", 1)
	insertThread(t, db, "sess-1", "t2", "channel", "agent-1", 2)

	for _, validRel := range []string{"child_of", "reply_to", "references"} {
		_, err := db.Exec(`
			INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
			VALUES ('sess-1', 't1', 't2', ?, 1706000000)
		`, validRel)
		require.NoError(t, err, "valid relation %q should be accepted", validRel)
		// Clean up for next iteration
		_, _ = db.Exec(`DELETE FROM fabric_dependencies WHERE session_id = 'sess-1' AND thread_id = 't1' AND depends_on_id = 't2' AND relation = ?`, validRel)
	}

	// Invalid relation type should fail
	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 't1', 't2', 'invalid_rel', 1706000000)
	`)
	require.Error(t, err, "CHECK constraint should reject invalid relation type")
}

// TestFabricGraph_InsertAndQuery verifies basic CRUD operations work on fabric tables.
func TestFabricGraph_InsertAndQuery(t *testing.T) {
	db := setupFabricDB(t)
	defer db.Close()

	// Insert a channel with all fields
	_, err := db.Exec(`
		INSERT INTO fabric_threads (
			session_id, id, type, created_at, created_by,
			content, kind, slug, title, purpose,
			name, media_type, size_bytes, storage_uri, sha256,
			mentions_json, participants_json, meta_json,
			seq, archived_at
		) VALUES (
			'sess-1', 'ch-general', 'channel', 1706000000, 'coordinator',
			'Welcome', 'info', 'general', 'General', 'General chat',
			NULL, NULL, NULL, NULL, NULL,
			'["worker-1","worker-2"]', '["coordinator","worker-1"]', '{"key":"value"}',
			1, NULL
		)
	`)
	require.NoError(t, err)

	// Query back and verify
	var (
		sessionID, id, typ, createdBy  string
		createdAt, seq                 int64
		content, kind, slug            *string
		title, purpose                 *string
		mentionsJSON, participantsJSON *string
		metaJSON                       *string
		archivedAt                     *int64
	)
	err = db.QueryRow(`
		SELECT session_id, id, type, created_at, created_by,
		       content, kind, slug, title, purpose,
		       mentions_json, participants_json, meta_json,
		       seq, archived_at
		FROM fabric_threads WHERE session_id = 'sess-1' AND id = 'ch-general'
	`).Scan(
		&sessionID, &id, &typ, &createdAt, &createdBy,
		&content, &kind, &slug, &title, &purpose,
		&mentionsJSON, &participantsJSON, &metaJSON,
		&seq, &archivedAt,
	)
	require.NoError(t, err)
	require.Equal(t, "sess-1", sessionID)
	require.Equal(t, "ch-general", id)
	require.Equal(t, "channel", typ)
	require.Equal(t, int64(1706000000), createdAt)
	require.Equal(t, "coordinator", createdBy)
	require.NotNil(t, content)
	require.Equal(t, "Welcome", *content)
	require.NotNil(t, slug)
	require.Equal(t, "general", *slug)
	require.NotNil(t, mentionsJSON)
	require.Equal(t, `["worker-1","worker-2"]`, *mentionsJSON)
	require.NotNil(t, metaJSON)
	require.Equal(t, `{"key":"value"}`, *metaJSON)
	require.Nil(t, archivedAt)

	// Insert a dependency
	_, err = db.Exec(`
		INSERT INTO fabric_threads (session_id, id, type, created_at, created_by, seq)
		VALUES ('sess-1', 'msg-1', 'message', 1706000001, 'worker-1', 2)
	`)
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO fabric_dependencies (session_id, thread_id, depends_on_id, relation, created_at)
		VALUES ('sess-1', 'msg-1', 'ch-general', 'child_of', 1706000001)
	`)
	require.NoError(t, err)

	// Query dependency back
	var threadID, dependsOnID, relation string
	var depCreatedAt int64
	err = db.QueryRow(`
		SELECT thread_id, depends_on_id, relation, created_at
		FROM fabric_dependencies WHERE session_id = 'sess-1' AND thread_id = 'msg-1'
	`).Scan(&threadID, &dependsOnID, &relation, &depCreatedAt)
	require.NoError(t, err)
	require.Equal(t, "msg-1", threadID)
	require.Equal(t, "ch-general", dependsOnID)
	require.Equal(t, "child_of", relation)
}

// TestFabricGraph_EmbeddedFiles verifies the new migration SQL files are embedded.
func TestFabricGraph_EmbeddedFiles(t *testing.T) {
	entries, err := embeddedMigrationsFS.ReadDir(".")
	require.NoError(t, err)

	fileNames := make(map[string]bool)
	for _, entry := range entries {
		fileNames[entry.Name()] = true
	}

	require.True(t, fileNames["000003_create_fabric_graph.up.sql"], "fabric graph up migration should be embedded")
	require.True(t, fileNames["000003_create_fabric_graph.down.sql"], "fabric graph down migration should be embedded")

	// Verify content is not empty
	upContent, err := embeddedMigrationsFS.ReadFile("000003_create_fabric_graph.up.sql")
	require.NoError(t, err)
	require.Contains(t, string(upContent), "CREATE TABLE fabric_threads")
	require.Contains(t, string(upContent), "CREATE TABLE fabric_dependencies")

	downContent, err := embeddedMigrationsFS.ReadFile("000003_create_fabric_graph.down.sql")
	require.NoError(t, err)
	require.Contains(t, string(downContent), "DROP TABLE")
}
