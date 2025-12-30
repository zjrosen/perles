// Package log provides structured logging for Perles.
// It wraps tea.LogToFile with structured fields (level, category, timestamp)
// and conditionally enables logging via --debug flag or PERLES_DEBUG env.
package log

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/zjrosen/perles/internal/pubsub"
)

// Level represents log severity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Category groups related log messages.
type Category string

const (
	CatBQL     Category = "bql"     // BQL parsing and execution
	CatDB      Category = "db"      // Database operations
	CatConfig  Category = "config"  // Configuration loading/saving
	CatWatcher Category = "watcher" // File watcher events
	CatUI      Category = "ui"      // UI component updates
	CatMode    Category = "mode"    // Mode controller events
	CatBeads   Category = "beads"   // Beads CLI interactions
	CatOrch    Category = "orch"    // Orchestration: coordinator, pool, amp, claude processes
	CatMCP     Category = "mcp"     // MCP server and protocol communication
	CatTree    Category = "tree"    // Tree view operations
	CatCache   Category = "cache"   // cache operations
)

// Logger provides structured logging.
type Logger struct {
	mu       sync.Mutex
	file     *os.File
	writer   io.Writer
	enabled  bool
	minLevel Level
	broker   *pubsub.Broker[string] // Pub/sub for log events
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the global logger.
// Returns a cleanup function to close the log file.
func Init(path string) (func(), error) {
	var initErr error
	once.Do(func() {
		defaultLogger, initErr = newLogger(path)
	})
	if initErr != nil {
		return nil, initErr
	}
	// Check if logger was initialized (handles case where once.Do already ran)
	if defaultLogger == nil {
		return nil, fmt.Errorf("logger initialization failed or already attempted")
	}
	return func() {
		if defaultLogger != nil && defaultLogger.file != nil {
			_ = defaultLogger.file.Close()
		}
	}, nil
}

// InitWithTeaLog uses tea.LogToFile for initialization.
func InitWithTeaLog(path string, prefix string) (func(), error) {
	f, err := tea.LogToFile(path, prefix)
	if err != nil {
		return nil, err
	}

	defaultLogger = &Logger{
		file:     f,
		writer:   f,
		enabled:  true,
		minLevel: LevelDebug,
		broker:   pubsub.NewBroker[string](),
	}

	return func() { _ = f.Close() }, nil
}

func newLogger(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G304: path is user-controlled debug log path
	if err != nil {
		return nil, err
	}

	return &Logger{
		file:     f,
		writer:   f,
		enabled:  true,
		minLevel: LevelDebug,
		broker:   pubsub.NewBroker[string](),
	}, nil
}

// SetEnabled toggles logging on/off.
func SetEnabled(enabled bool) {
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.enabled = enabled
		defaultLogger.mu.Unlock()
	}
}

// SetMinLevel sets the minimum log level.
func SetMinLevel(level Level) {
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.minLevel = level
		defaultLogger.mu.Unlock()
	}
}

// Debug logs at debug level.
func Debug(cat Category, msg string, fields ...any) {
	log(LevelDebug, cat, msg, fields...)
}

// Info logs at info level.
func Info(cat Category, msg string, fields ...any) {
	log(LevelInfo, cat, msg, fields...)
}

// Warn logs at warning level.
func Warn(cat Category, msg string, fields ...any) {
	log(LevelWarn, cat, msg, fields...)
}

// Error logs at error level.
func Error(cat Category, msg string, fields ...any) {
	log(LevelError, cat, msg, fields...)
}

// ErrorErr logs an error with the error value.
func ErrorErr(cat Category, msg string, err error, fields ...any) {
	if err != nil {
		fields = append(fields, "error", err.Error())
	} else {
		fields = append(fields, "error", "<nil>")
	}
	log(LevelError, cat, msg, fields...)
}

func log(level Level, cat Category, msg string, fields ...any) {
	if defaultLogger == nil || !defaultLogger.enabled {
		return
	}
	if level < defaultLogger.minLevel {
		return
	}

	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()

	// Format: 2025-12-06T10:45:00 [ERROR] [bql] message key=value key2=value2
	timestamp := time.Now().Format("2006-01-02T15:04:05")
	entry := fmt.Sprintf("%s [%s] [%s] %s", timestamp, level, cat, msg)

	// Append fields (key=value pairs)
	for i := 0; i+1 < len(fields); i += 2 {
		key := fields[i]
		value := fields[i+1]
		entry += fmt.Sprintf(" %v=%v", key, value)
	}
	// Handle odd field count - append orphan key with no value
	if len(fields)%2 != 0 {
		entry += fmt.Sprintf(" %v=<missing>", fields[len(fields)-1])
	}
	entry += "\n"

	// Write to file
	if defaultLogger.writer != nil {
		_, _ = defaultLogger.writer.Write([]byte(entry))
	}

	// Publish event to subscribers (non-blocking)
	if defaultLogger.broker != nil {
		defaultLogger.broker.Publish(pubsub.CreatedEvent, entry)
	}
}

// LogEvent is a pubsub event containing a log entry.
type LogEvent = pubsub.Event[string]

// LogListener wraps a continuous listener for log events.
type LogListener = pubsub.ContinuousListener[string]

// NewListener creates a new log event listener.
// The listener is automatically cleaned up when the context is cancelled.
func NewListener(ctx context.Context) *LogListener {
	if defaultLogger == nil || defaultLogger.broker == nil {
		return nil
	}
	return pubsub.NewContinuousListener(ctx, defaultLogger.broker)
}
