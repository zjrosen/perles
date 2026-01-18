package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/zjrosen/perles/internal/log"
)

// CommandFactoryFunc creates an exec.Cmd for testing purposes.
// It receives the context, executable path, and arguments.
type CommandFactoryFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

// SpawnBuilder provides a fluent API for spawning headless AI processes.
// It consolidates the common spawn boilerplate (context setup, pipe creation,
// process start) while preserving provider flexibility.
type SpawnBuilder struct {
	ctx              context.Context
	timeout          time.Duration
	execPath         string
	args             []string
	workDir          string
	sessionRef       string
	env              []string
	parser           EventParser
	providerName     string
	captureStderr    bool
	needsStdin       bool
	onInitEventFn    OnInitEventFunc
	sessionExtractor SessionExtractorFunc
	commandFactory   CommandFactoryFunc
}

// NewSpawnBuilder creates a new SpawnBuilder with the given context.
func NewSpawnBuilder(ctx context.Context) *SpawnBuilder {
	return &SpawnBuilder{
		ctx:          ctx,
		providerName: "unknown",
	}
}

// WithExecutable sets the executable path and arguments.
func (b *SpawnBuilder) WithExecutable(path string, args []string) *SpawnBuilder {
	b.execPath = path
	b.args = args
	return b
}

// WithWorkDir sets the working directory for the process.
func (b *SpawnBuilder) WithWorkDir(dir string) *SpawnBuilder {
	b.workDir = dir
	return b
}

// WithSessionRef sets the initial session reference (session ID, thread ID, etc).
func (b *SpawnBuilder) WithSessionRef(ref string) *SpawnBuilder {
	b.sessionRef = ref
	return b
}

// WithTimeout sets the process timeout. If d is 0 or negative,
// a cancel-only context is created instead of a timeout context.
func (b *SpawnBuilder) WithTimeout(d time.Duration) *SpawnBuilder {
	b.timeout = d
	return b
}

// WithParser sets the EventParser for parsing process output.
// This is a required field - Build() will fail if not set.
func (b *SpawnBuilder) WithParser(p EventParser) *SpawnBuilder {
	b.parser = p
	return b
}

// WithEnv sets additional environment variables to append to os.Environ().
// Variables are in the format "KEY=VALUE".
func (b *SpawnBuilder) WithEnv(env []string) *SpawnBuilder {
	b.env = env
	return b
}

// WithProviderName sets the provider name for logging and error messages.
func (b *SpawnBuilder) WithProviderName(name string) *SpawnBuilder {
	b.providerName = name
	return b
}

// WithStderrCapture enables stderr line capture for error messages.
func (b *SpawnBuilder) WithStderrCapture(capture bool) *SpawnBuilder {
	b.captureStderr = capture
	return b
}

// WithStdin enables stdin pipe creation.
// After Build(), use BaseProcess.Stdin() to access the pipe.
func (b *SpawnBuilder) WithStdin(enabled bool) *SpawnBuilder {
	b.needsStdin = enabled
	return b
}

// WithOnInitEvent sets a callback for init events.
// This is used by providers like Claude to extract additional info (e.g., mainModel).
func (b *SpawnBuilder) WithOnInitEvent(fn OnInitEventFunc) *SpawnBuilder {
	b.onInitEventFn = fn
	return b
}

// WithSessionExtractor sets a custom session extraction function.
// This overrides the parser's ExtractSessionRef method if set.
// Used by providers like Claude that need custom session ID extraction logic.
func (b *SpawnBuilder) WithSessionExtractor(fn SessionExtractorFunc) *SpawnBuilder {
	b.sessionExtractor = fn
	return b
}

// WithCommandFactory sets a custom command factory for testing.
// This allows unit tests to mock exec.Command without spawning real processes.
func (b *SpawnBuilder) WithCommandFactory(fn CommandFactoryFunc) *SpawnBuilder {
	b.commandFactory = fn
	return b
}

// Build validates the configuration, creates the process, and starts it.
// Returns the configured BaseProcess or an error.
//
// Build performs the following steps:
//  1. Validates required fields (execPath, parser)
//  2. Creates context with timeout (if configured) or cancel-only
//  3. Creates exec.Cmd (using commandFactory if set)
//  4. Creates pipes (stdin if needsStdin, stdout, stderr)
//  5. Delegates to NewBaseProcess() with configured options
//  6. Starts the process and goroutines
//
// On error, all created resources are cleaned up.
func (b *SpawnBuilder) Build() (*BaseProcess, error) {
	// Validate required fields
	if b.execPath == "" {
		return nil, fmt.Errorf("spawn builder: executable path is required")
	}
	if b.parser == nil {
		return nil, fmt.Errorf("spawn builder: parser is required")
	}

	// Create context with timeout or cancel-only
	var procCtx context.Context
	var cancel context.CancelFunc
	if b.timeout > 0 {
		procCtx, cancel = context.WithTimeout(b.ctx, b.timeout)
	} else {
		procCtx, cancel = context.WithCancel(b.ctx)
	}

	// Track resources for cleanup on error
	var cmd *exec.Cmd
	var stdin io.WriteCloser
	var stdout io.ReadCloser
	var stderr io.ReadCloser

	cleanup := func() {
		cancel()
		if stdin != nil {
			_ = stdin.Close()
		}
		if stdout != nil {
			_ = stdout.Close()
		}
		if stderr != nil {
			_ = stderr.Close()
		}
	}

	// Create command
	if b.commandFactory != nil {
		cmd = b.commandFactory(procCtx, b.execPath, b.args...)
	} else {
		// #nosec G204 -- args are built from Config struct, not user input
		cmd = exec.CommandContext(procCtx, b.execPath, b.args...)
	}
	cmd.Dir = b.workDir

	// Set environment variables (append to os.Environ())
	if len(b.env) > 0 {
		cmd.Env = append(os.Environ(), b.env...)
	}

	// Create stdin pipe if needed
	if b.needsStdin {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("spawn builder: failed to create stdin pipe: %w", err)
		}
	}

	// Create stdout pipe
	var err error
	stdout, err = cmd.StdoutPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("spawn builder: failed to create stdout pipe: %w", err)
	}

	// Create stderr pipe
	stderr, err = cmd.StderrPipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("spawn builder: failed to create stderr pipe: %w", err)
	}

	// Build BaseProcess options
	opts := []BaseProcessOption{
		WithEventParser(b.parser),
		WithStderrCapture(b.captureStderr),
		WithProviderName(b.providerName),
	}
	if b.onInitEventFn != nil {
		opts = append(opts, WithOnInitEvent(b.onInitEventFn))
	}
	// WithSessionExtractor must be added AFTER WithEventParser to override it
	if b.sessionExtractor != nil {
		opts = append(opts, WithSessionExtractor(b.sessionExtractor))
	}

	// Create BaseProcess
	bp := NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		b.workDir,
		opts...,
	)

	// Set stdin if created
	if stdin != nil {
		bp.SetStdin(stdin)
	}

	// Set initial session reference if provided
	if b.sessionRef != "" {
		bp.SetSessionRef(b.sessionRef)
	}

	// Log spawn attempt
	log.Debug(log.CatOrch, "Spawning process",
		"subsystem", b.providerName,
		"execPath", b.execPath,
		"workDir", b.workDir)

	// Start the process
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("spawn builder: failed to start %s process: %w", b.providerName, err)
	}

	log.Debug(log.CatOrch, "Process started",
		"subsystem", b.providerName,
		"pid", cmd.Process.Pid)

	bp.SetStatus(StatusRunning)

	// Start output parser goroutines
	bp.StartGoroutines()

	return bp, nil
}
