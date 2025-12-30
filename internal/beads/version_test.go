package beads

import (
	"database/sql"
	"testing"

	"github.com/zjrosen/perles/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal", "0.30.6", "0.30.6", 0},
		{"a less than b patch", "0.30.5", "0.30.6", -1},
		{"a greater than b patch", "0.30.7", "0.30.6", 1},
		{"a less than b minor not lexicographic", "0.9.0", "0.30.0", -1},
		{"a greater than b major", "1.0.0", "0.99.99", 1},
		{"v prefix on a", "v0.30.6", "0.30.6", 0},
		{"v prefix on b", "0.30.6", "v0.30.6", 0},
		{"v prefix on both", "v0.30.6", "v0.30.6", 0},
		{"a less than b major", "0.30.6", "1.0.0", -1},
		{"missing patch treated as 0", "0.30", "0.30.0", 0},
		{"missing minor and patch treated as 0", "1", "1.0.0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			require.Equal(t, tt.want, got, "CompareVersions(%q, %q)", tt.a, tt.b)
		})
	}
}

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		name    string
		current string
		wantErr bool
	}{
		{"exact match", MinBeadsVersion, false},
		{"newer patch", "0.41.1", false},
		{"newer minor", "0.42.0", false},
		{"newer major", "1.0.0", false},
		{"older patch", "0.40.9", true},
		{"older minor", "0.40.0", true},
		{"much older", "0.9.0", true},
		{"with v prefix newer", "v1.0.0", false},
		{"with v prefix older", "v0.29.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckVersion(tt.current)
			if tt.wantErr {
				require.Error(t, err, "CheckVersion(%q) should return error", tt.current)
				require.Contains(t, err.Error(), MinBeadsVersion)
				require.Contains(t, err.Error(), tt.current)
			} else {
				require.NoError(t, err, "CheckVersion(%q) should not return error", tt.current)
			}
		})
	}
}

func TestClient_Version(t *testing.T) {
	db := testutil.NewTestDB(t)
	defer func() { _ = db.Close() }()

	// Add metadata table (not in standard test schema)
	_, err := db.Exec(`CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT)`)
	require.NoError(t, err)

	t.Run("returns version when present", func(t *testing.T) {
		_, err := db.Exec(`INSERT OR REPLACE INTO metadata (key, value) VALUES ('bd_version', '0.31.0')`)
		require.NoError(t, err)

		client := &Client{db: db}
		version, err := client.Version()
		require.NoError(t, err)
		require.Equal(t, "0.31.0", version)
	})

	t.Run("returns error when bd_version missing", func(t *testing.T) {
		_, err := db.Exec(`DELETE FROM metadata WHERE key = 'bd_version'`)
		require.NoError(t, err)

		client := &Client{db: db}
		_, err = client.Version()
		require.Error(t, err)
		require.Contains(t, err.Error(), "reading bd_version from metadata")
	})
}

func TestClient_Version_NoMetadataTable(t *testing.T) {
	// Create a minimal database without metadata table
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	client := &Client{db: db}
	_, err = client.Version()
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading bd_version from metadata")
}
