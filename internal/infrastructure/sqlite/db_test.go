package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/sessions/domain"
)

// TestNewDB_CreatesDirectory verifies that NewDB creates the parent directory if missing.
func TestNewDB_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "nested", "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed even with nested non-existent directories")
	defer db.Close()

	// Verify the directory was created
	info, err := os.Stat(filepath.Dir(dbPath))
	require.NoError(t, err, "Directory should exist after NewDB")
	require.True(t, info.IsDir(), "Should be a directory")

	// Verify directory permissions are 0700 (Unix only - Windows doesn't support Unix permissions)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0700), info.Mode().Perm(), "Directory should have 0700 permissions")
	}
}

// TestNewDB_CreatesDatabaseFile verifies that NewDB creates the database file on first run.
func TestNewDB_CreatesDatabaseFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	// Verify the database file was created
	info, err := os.Stat(dbPath)
	require.NoError(t, err, "Database file should exist after NewDB")
	require.False(t, info.IsDir(), "Should be a file, not a directory")
}

// TestNewDB_RunsMigrations verifies that NewDB runs migrations and creates the sessions table.
func TestNewDB_RunsMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	// Verify sessions table exists by querying it
	var tableName string
	err = db.conn.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='sessions'",
	).Scan(&tableName)
	require.NoError(t, err, "sessions table should exist after migrations")
	require.Equal(t, "sessions", tableName)
}

// TestNewDB_PreMigrationBackup verifies that a .bak file is created before migrations
// when an existing database file is present.
func TestNewDB_PreMigrationBackup(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create initial database
	db1, err := NewDB(dbPath)
	require.NoError(t, err, "First NewDB should succeed")

	// Insert test data to verify backup content
	_, err = db1.conn.Exec(
		"INSERT INTO sessions (guid, project, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		"test-guid", "test-project", "running", 1000, 1000,
	)
	require.NoError(t, err, "Should be able to insert test data")
	db1.Close()

	// Open database again - this should create a backup
	db2, err := NewDB(dbPath)
	require.NoError(t, err, "Second NewDB should succeed")
	defer db2.Close()

	// Verify backup file exists
	backupPath := dbPath + ".bak"
	info, err := os.Stat(backupPath)
	require.NoError(t, err, "Backup file should exist after second NewDB")
	require.False(t, info.IsDir(), "Backup should be a file")
	require.Greater(t, info.Size(), int64(0), "Backup file should have content")
}

// TestNewDB_WALMode verifies that WAL mode is enabled via PRAGMA query.
func TestNewDB_WALMode(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	// Query journal_mode pragma
	var journalMode string
	err = db.conn.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err, "Should be able to query journal_mode")
	require.Equal(t, "wal", journalMode, "Journal mode should be WAL")
}

// TestNewDB_ForeignKeys verifies that foreign keys are enabled via PRAGMA query.
func TestNewDB_ForeignKeys(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	// Query foreign_keys pragma
	var foreignKeys int
	err = db.conn.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys)
	require.NoError(t, err, "Should be able to query foreign_keys")
	require.Equal(t, 1, foreignKeys, "Foreign keys should be enabled (1)")
}

// TestNewDB_BusyTimeout verifies that busy timeout is set to 5000ms via PRAGMA query.
func TestNewDB_BusyTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	// Query busy_timeout pragma
	var busyTimeout int
	err = db.conn.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout)
	require.NoError(t, err, "Should be able to query busy_timeout")
	require.Equal(t, 5000, busyTimeout, "Busy timeout should be 5000ms")
}

// TestDB_Close verifies that connection closes cleanly.
func TestDB_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")

	// Close should succeed
	err = db.Close()
	require.NoError(t, err, "Close should succeed")

	// After close, the connection should be closed (ping fails)
	err = db.conn.Ping()
	require.Error(t, err, "Ping should fail after Close")
}

// TestDB_SessionRepository verifies that SessionRepository returns an implementation
// that satisfies the domain.SessionRepository interface.
func TestDB_SessionRepository(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	repo := db.SessionRepository()
	require.NotNil(t, repo, "SessionRepository should not return nil")

	// Verify it implements the interface
	var _ domain.SessionRepository = repo
}

// TestDB_Connection verifies that Connection returns the underlying *sql.DB.
func TestDB_Connection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDB(dbPath)
	require.NoError(t, err, "NewDB should succeed")
	defer db.Close()

	conn := db.Connection()
	require.NotNil(t, conn, "Connection should not return nil")
	require.IsType(t, (*sql.DB)(nil), conn, "Connection should return *sql.DB")

	// Connection should be usable
	err = conn.Ping()
	require.NoError(t, err, "Connection should be pingable")
}

// TestNewDB_MultipleCalls verifies that opening the same database twice is safe.
func TestNewDB_MultipleCalls(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First connection
	db1, err := NewDB(dbPath)
	require.NoError(t, err, "First NewDB should succeed")
	defer db1.Close()

	// Second connection to the same database
	db2, err := NewDB(dbPath)
	require.NoError(t, err, "Second NewDB should succeed (WAL mode allows concurrent access)")
	defer db2.Close()

	// Both connections should be usable
	err = db1.conn.Ping()
	require.NoError(t, err, "First connection should still work")

	err = db2.conn.Ping()
	require.NoError(t, err, "Second connection should work")

	// Both can query
	var count1, count2 int
	err = db1.conn.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count1)
	require.NoError(t, err, "First connection should be able to query")

	err = db2.conn.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count2)
	require.NoError(t, err, "Second connection should be able to query")
}

// TestNewDB_InvalidPath verifies that NewDB returns an error for invalid paths.
func TestNewDB_InvalidPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific restricted path test")
	}

	// Path in a restricted directory (root)
	invalidPath := "/root/perles-test-db.sqlite"

	_, err := NewDB(invalidPath)
	require.Error(t, err, "NewDB should fail for path in restricted directory")
}
