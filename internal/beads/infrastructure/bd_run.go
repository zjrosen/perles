package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

// errBDTimeout is the cancellation cause recorded when perles' own per-call
// timeout kills bd, as opposed to the caller's context ending.
var errBDTimeout = errors.New("bd timeout")

// BDRequest describes one bd invocation. The zero value of each optional
// field keeps the default behaviour.
type BDRequest struct {
	// Args are bd's arguments, e.g. {"--readonly", "export", "--all"}.
	Args []string
	// Stdin is written to bd's standard input. Empty means no input.
	Stdin string
	// Env holds extra KEY=VALUE entries, merged after bdEnviron: each one
	// replaces any inherited entry with the same key, and a later entry
	// replaces an earlier one. BD_JSON_ENVELOPE and, when the runner has a
	// beads dir, BEADS_DIR stay pinned, so entries for them are ignored.
	Env []string
	// Timeout bounds this invocation; zero or negative means the
	// executor's timeout (defaultBDTimeout unless overridden).
	Timeout time.Duration
}

// BDRunner runs one bd invocation and returns its stdout as bd wrote it.
// On failure the error is the translated *bdError (see newBDError), which
// names the per-call timeout when bd was killed after it. When ctx ends
// first, bd is killed and the error wraps ctx's cause.
//
// NewBDRunner returns the runner over the real bd; tests inject fakes that
// observe the request.
type BDRunner func(ctx context.Context, req BDRequest) (string, error)

// NewBDRunner returns a BDRunner that runs the bd on PATH in workDir with
// the environment from bdEnviron for beadsDir. A relative beadsDir is made
// absolute against perles' working directory, as in NewBDExecutor.
func NewBDRunner(workDir, beadsDir string) BDRunner {
	return NewBDExecutor(workDir, beadsDir).run
}

// run implements BDRunner over execBDRequest and newBDError.
func (e *BDExecutor) run(ctx context.Context, req BDRequest) (string, error) {
	out := e.execBDRequest(ctx, req)
	if out.failed() {
		return "", newBDError(req.Args, out, e.requestTimeout(req))
	}
	return out.Stdout, nil
}

// execBD runs the real bd binary with args and the executor's defaults: no
// stdin, no extra environment and the executor's timeout.
func (e *BDExecutor) execBD(args []string) bdOutput {
	return e.execBDRequest(context.Background(), BDRequest{Args: args})
}

// execBDRequest runs the real bd binary once, in workDir, with the
// environment from bdEnviron plus req.Env. bd is killed when the request's
// timeout elapses (TimedOut is set) or when ctx ends (Err wraps ctx's cause).
func (e *BDExecutor) execBDRequest(parent context.Context, req BDRequest) bdOutput {
	ctx, cancel := context.WithTimeoutCause(parent, e.requestTimeout(req), errBDTimeout)
	defer cancel()

	//nolint:gosec // G204: args come from controlled sources
	cmd := exec.CommandContext(ctx, "bd", req.Args...)
	if e.workDir != "" {
		cmd.Dir = e.workDir
	}
	cmd.Env = mergeBDEnv(bdEnviron(os.Environ(), e.beadsDir), req.Env, e.beadsDir)
	cmd.WaitDelay = bdWaitDelay
	if req.Stdin != "" {
		cmd.Stdin = strings.NewReader(req.Stdin)
	}

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
		switch cause := context.Cause(ctx); {
		case errors.Is(cause, errBDTimeout):
			out.TimedOut = true
		case cause != nil && !errors.Is(err, cause):
			// The caller's context ended (e.g. the backend closed): keep
			// that visible to errors.Is behind the kill's exec error.
			out.Err = fmt.Errorf("%w: %w", cause, err)
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

// requestTimeout is req's timeout, or the executor's when req sets none.
func (e *BDExecutor) requestTimeout(req BDRequest) time.Duration {
	if req.Timeout > 0 {
		return req.Timeout
	}
	return e.effectiveTimeout()
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
		if isPinnedBDEnvKey(envKey(kv), beadsDir) {
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

// mergeBDEnv adds extra KEY=VALUE entries to env, the output of bdEnviron
// for beadsDir. An extra entry replaces any entry in env with the same key,
// and a later extra entry replaces an earlier one, so no key appears twice
// (bdEnviron's policy: duplicates are removed rather than relying on
// last-one-wins). Extra entries for the keys bdEnviron pins are dropped.
func mergeBDEnv(env, extra []string, beadsDir string) []string {
	if len(extra) == 0 {
		return env
	}
	sameKey := func(key string) func(string) bool {
		return func(kv string) bool { return strings.EqualFold(envKey(kv), key) }
	}

	kept := make([]string, 0, len(extra))
	for i, kv := range extra {
		key := envKey(kv)
		if isPinnedBDEnvKey(key, beadsDir) || slices.ContainsFunc(extra[i+1:], sameKey(key)) {
			continue
		}
		kept = append(kept, kv)
	}

	merged := make([]string, 0, len(env)+len(kept))
	for _, kv := range env {
		if slices.ContainsFunc(kept, sameKey(envKey(kv))) {
			continue
		}
		merged = append(merged, kv)
	}
	return append(merged, kept...)
}

// isPinnedBDEnvKey reports whether bdEnviron sets key itself for beadsDir:
// BD_JSON_ENVELOPE always, BEADS_DIR when beadsDir is set. Keys compare
// case-insensitively, as Windows environment keys do.
func isPinnedBDEnvKey(key, beadsDir string) bool {
	return strings.EqualFold(key, "BD_JSON_ENVELOPE") ||
		(beadsDir != "" && strings.EqualFold(key, "BEADS_DIR"))
}

// envKey is the key of a KEY=VALUE environment entry.
func envKey(kv string) string {
	key, _, _ := strings.Cut(kv, "=")
	return key
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
