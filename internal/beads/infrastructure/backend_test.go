package infrastructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectBackend_NoMetadataJSON(t *testing.T) {
	beadsDir := t.TempDir()

	backend, err := DetectBackend(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "sqlite", backend)
}

func TestDetectBackend_SQLiteExplicit(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend": "sqlite"}`)

	backend, err := DetectBackend(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "sqlite", backend)
}

func TestDetectBackend_EmptyBackend(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{}`)

	backend, err := DetectBackend(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "sqlite", backend)
}

func TestDetectBackend_Dolt(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend": "dolt", "dolt_database": "beads"}`)

	backend, err := DetectBackend(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "dolt", backend)
}

func TestDetectBackend_UnsupportedBackend(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend": "postgres"}`)

	_, err := DetectBackend(beadsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported backend type")
}

func TestDetectBackend_InvalidJSON(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `not valid json`)

	_, err := DetectBackend(beadsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing metadata.json")
}

func TestLoadMetadata_DoltDefaultsDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend": "dolt"}`)

	meta, err := LoadMetadata(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "dolt", meta.Backend)
	require.Equal(t, "beads", meta.DoltDatabase)
}

func TestLoadMetadata_DoltCustomDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	writeMetadata(t, beadsDir, `{"backend": "dolt", "dolt_database": "mydb"}`)

	meta, err := LoadMetadata(beadsDir)
	require.NoError(t, err)
	require.Equal(t, "dolt", meta.Backend)
	require.Equal(t, "mydb", meta.DoltDatabase)
}

func writeMetadata(t *testing.T, beadsDir, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(content), 0600)
	require.NoError(t, err)
}
