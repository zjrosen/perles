package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultBDTimeout bounds a single bd invocation. bd waits up to 30s on a
// workspace gate, and the first write after a bd upgrade can spend 10-25s
// migrating the database, so the limit leaves room for both before perles
// gives up on the command.
const defaultBDTimeout = 90 * time.Second

// bdWaitDelay is how long to wait for bd's output pipes to close after the
// process is killed on timeout (a child process could still hold them).
const bdWaitDelay = 5 * time.Second

// bdOutput is the raw result of one bd invocation.
type bdOutput struct {
	Stdout string
	Stderr string
	// ExitCode is bd's exit status: 0 on success, -1 when bd did not run to
	// completion (not found, killed on timeout).
	ExitCode int
	// Err is the exec error, nil when bd exited 0.
	Err error
	// TimedOut reports that perles killed bd after the timeout.
	TimedOut bool
}

// failed reports whether the invocation did not succeed.
func (o bdOutput) failed() bool {
	return o.Err != nil || o.ExitCode != 0 || o.TimedOut
}

// execBD runs the real bd binary with a timeout, in workDir, with the
// environment from bdEnviron.
func (e *BDExecutor) execBD(args []string) bdOutput {
	ctx, cancel := context.WithTimeout(context.Background(), e.effectiveTimeout())
	defer cancel()

	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.CommandContext(ctx, "bd", args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = bdEnviron(os.Environ(), e.beadsDir)
	cmd.WaitDelay = bdWaitDelay

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := bdOutput{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
	if err != nil {
		out.ExitCode = -1
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			out.ExitCode = exitErr.ExitCode()
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			out.TimedOut = true
		}
	}
	return out
}

func (e *BDExecutor) effectiveTimeout() time.Duration {
	if e.timeout > 0 {
		return e.timeout
	}
	return defaultBDTimeout
}

// bdEnviron builds bd's environment from base (normally os.Environ()).
//
// It drops any inherited BD_JSON_ENVELOPE (bd >= 1.3 wraps every --json
// payload as {"schema_version":1,"data":...} when it is "1"; any other value
// disables the envelope, so it is pinned to "0") and, when beadsDir is set,
// any inherited BEADS_DIR, replacing it with beadsDir. Duplicate keys are
// removed rather than relying on last-one-wins.
func bdEnviron(base []string, beadsDir string) []string {
	env := make([]string, 0, len(base)+2)
	for _, kv := range base {
		key, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(key, "BD_JSON_ENVELOPE") {
			continue
		}
		if beadsDir != "" && strings.EqualFold(key, "BEADS_DIR") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "BD_JSON_ENVELOPE=0")
	if beadsDir != "" {
		env = append(env, "BEADS_DIR="+beadsDir)
	}
	return env
}

// absBeadsDir makes beadsDir absolute so bd resolves it the same way perles
// did, regardless of the working directory bd runs in. It returns the input
// unchanged when it is empty or cannot be made absolute.
func absBeadsDir(beadsDir string) string {
	if beadsDir == "" || filepath.IsAbs(beadsDir) {
		return beadsDir
	}
	abs, err := filepath.Abs(beadsDir)
	if err != nil {
		return beadsDir
	}
	return abs
}
