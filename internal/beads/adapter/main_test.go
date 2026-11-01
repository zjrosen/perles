package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBDStderrEnv turns this test binary into a bd that refuses every
// command: started with it set, it prints the variable's value on stderr
// and exits 1 (see installFailingBD).
const fakeBDStderrEnv = "PERLES_ADAPTER_TEST_FAKE_BD_STDERR"

func TestMain(m *testing.M) {
	if stderr, ok := os.LookupEnv(fakeBDStderrEnv); ok {
		fmt.Fprintln(os.Stderr, stderr)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// installFailingBD puts a bd that prints stderr and exits 1 alone on PATH
// for the rest of the test. It is a copy of this test binary, run with
// fakeBDStderrEnv set, so unlike a shell script it runs on every OS.
func installFailingBD(t *testing.T, stderr string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	bin, err := os.ReadFile(self)
	require.NoError(t, err)
	name := "bd"
	if runtime.GOOS == "windows" {
		name = "bd.exe"
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), bin, 0o755)) //nolint:gosec // test executable
	t.Setenv("PATH", dir)
	t.Setenv(fakeBDStderrEnv, stderr)
}
