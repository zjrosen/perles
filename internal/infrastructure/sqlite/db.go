// Package sqlite provides SQLite database infrastructure for Perles.
// It handles connection lifecycle, migrations, and repository implementations.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zjrosen/perles/internal/infrastructure/migrations"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/sessions/domain"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// DB manages the SQLite database connection for Perles.
// It provides connection lifecycle management, automatic migrations,
// and access to repository implementations.
type DB struct {
	conn *sql.DB
	path string
}

// NewDB opens a database connection, configures pragmas, and runs migrations.
// Creates the parent directory if it doesn't exist.
// If an existing database file is present, creates a backup at {path}.bak
// before running migrations.
//
// Example:
//
//	db, err := sqlite.NewDB("~/.perles/perles.db")
//	if err != nil {
//	    return err
//	}
//	defer db.Close()
func NewDB(path string) (*DB, error) {
	log.Debug(log.CatDB, "Opening database", "path", path)

	// Create parent directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.ErrorErr(log.CatDB, "Failed to create database directory", err, "path", dir)
		return nil, fmt.Errorf("failed to create database directory %s: %w", dir, err)
	}

	// Pre-migration backup: copy existing DB to {path}.bak
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".bak"
		if err := copyFile(path, backupPath); err != nil {
			log.ErrorErr(log.CatDB, "Failed to create pre-migration backup", err, "path", path, "backup", backupPath)
			return nil, fmt.Errorf("failed to create pre-migration backup: %w", err)
		}
		log.Debug(log.CatDB, "Created pre-migration backup", "backup", backupPath)
	}

	// Open connection with ncruces driver
	conn, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		log.ErrorErr(log.CatDB, "Failed to open database", err, "path", path)
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err := conn.PingContext(context.Background()); err != nil {
		_ = conn.Close()
		log.ErrorErr(log.CatDB, "Failed to ping database", err, "path", path)
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure WAL mode for better concurrent access
	if _, err := conn.ExecContext(context.Background(), "PRAGMA journal_mode=WAL"); err != nil {
		_ = conn.Close()
		log.ErrorErr(log.CatDB, "Failed to enable WAL mode", err)
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign key constraints
	if _, err := conn.ExecContext(context.Background(), "PRAGMA foreign_keys=ON"); err != nil {
		_ = conn.Close()
		log.ErrorErr(log.CatDB, "Failed to enable foreign keys", err)
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set busy timeout to 5000ms for better concurrency handling
	if _, err := conn.ExecContext(context.Background(), "PRAGMA busy_timeout=5000"); err != nil {
		_ = conn.Close()
		log.ErrorErr(log.CatDB, "Failed to set busy timeout", err)
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Run migrations
	if err := migrations.RunMigrations(conn); err != nil {
		_ = conn.Close()
		log.ErrorErr(log.CatDB, "Failed to run migrations", err)
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Info(log.CatDB, "Database initialized", "path", path)

	return &DB{
		conn: conn,
		path: path,
	}, nil
}

// Close releases database resources.
func (db *DB) Close() error {
	if db.conn != nil {
		log.Debug(log.CatDB, "Closing database", "path", db.path)
		return db.conn.Close()
	}
	return nil
}

// SessionRepository returns a SessionRepository instance using this connection.
// The repository implementation is in session_repository.go.
func (db *DB) SessionRepository() domain.SessionRepository {
	return newSessionRepository(db.conn)
}

// Connection returns the underlying *sql.DB for testing purposes.
func (db *DB) Connection() *sql.DB {
	return db.conn
}

// copyFile copies a file from src to dst.
// If dst exists, it will be overwritten.
// Returns error if copy fails or if closing the destination file fails
// (to ensure backup integrity on disk full or permission errors).
func copyFile(src, dst string) (retErr error) {
	sourceFile, err := os.Open(src) //nolint:gosec // G304: src is the database path, controlled by application
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close source file: %w", closeErr)
		}
	}()

	// Get source file permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, sourceInfo.Mode()) //nolint:gosec // G304: dst is backup path derived from database path
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := destFile.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close backup file: %w", closeErr)
		}
	}()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
