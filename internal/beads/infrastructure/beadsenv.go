package infrastructure

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/subosito/gotenv"

	"github.com/zjrosen/perles/internal/log"
)

// loadBeadsEnvFile imports per-project beads settings from <beadsDir>/.env,
// the file bd loads before every command (bd >= 1.0.0, GH#2520) so a
// workspace can keep its Dolt user, password, port or mode there.
//
// Like bd, it never overrides a variable that is already set. Unlike bd, it
// only imports BEADS_* and BD_* keys: perles passes its environment on to AI
// agent processes, so unrelated secrets in the file must not leak into them.
// BEADS_DIR is skipped because perles has already resolved the directory.
//
// It returns the keys it set, for logging and tests.
func loadBeadsEnvFile(beadsDir string) []string {
	if beadsDir == "" {
		return nil
	}
	path := filepath.Join(beadsDir, ".env")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	values, err := gotenv.Read(path)
	if err != nil {
		log.Debug(log.CatDB, "Ignoring unreadable beads .env file", "path", path, "error", err)
		return nil
	}
	var set []string
	for key, value := range values {
		if !importableBeadsEnvKey(key) {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			continue
		}
		set = append(set, key)
	}
	if len(set) > 0 {
		log.Debug(log.CatDB, "Loaded beads settings from .env", "path", path, "keys", strings.Join(set, ","))
	}
	return set
}

func importableBeadsEnvKey(key string) bool {
	if key == "BEADS_DIR" {
		return false
	}
	return strings.HasPrefix(key, "BEADS_") || strings.HasPrefix(key, "BD_")
}
