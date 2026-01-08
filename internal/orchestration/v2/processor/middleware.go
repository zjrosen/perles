// Package processor provides middleware components for the command processor.
// Middleware wraps command handlers to add cross-cutting concerns like
// logging, deduplication, and timeout enforcement.
package processor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
)

// Middleware wraps a CommandHandler to add additional behavior.
// Middleware functions are composed using ChainMiddleware.
type Middleware func(CommandHandler) CommandHandler

// ChainMiddleware applies middlewares to a handler in reverse order.
// The first middleware in the list will be the outermost wrapper.
// For example: ChainMiddleware(handler, logging, dedup, timeout)
// Results in: logging(dedup(timeout(handler)))
func ChainMiddleware(handler CommandHandler, middlewares ...Middleware) CommandHandler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// ===========================================================================
// Logging Middleware
// ===========================================================================

// LoggingMiddlewareConfig configures the logging middleware.
type LoggingMiddlewareConfig struct {
	// Reserved for future configuration options
}

// NewLoggingMiddleware creates a middleware that logs command execution.
func NewLoggingMiddleware(cfg LoggingMiddlewareConfig) Middleware {
	return func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			start := time.Now()

			// Extract trace ID if available
			traceID := ""
			if bc, ok := cmd.(*command.BaseCommand); ok {
				traceID = bc.TraceID()
			} else if hasTraceID, ok := cmd.(interface{ TraceID() string }); ok {
				traceID = hasTraceID.TraceID()
			}

			// Extract source if available
			source := ""
			if hasSource, ok := cmd.(interface{ Source() command.CommandSource }); ok {
				source = string(hasSource.Source())
			}

			// Execute the handler
			result, err := next.Handle(ctx, cmd)

			// Calculate duration
			duration := time.Since(start)

			// Log after processing
			if err != nil {
				log.Error(log.CatCommands, "command failed",
					"command_id", cmd.ID(),
					"command_type", cmd.Type().String(),
					"trace_id", traceID,
					"duration", duration,
					"source", source,
					"error", err.Error(),
				)
			} else if result != nil && !result.Success {
				errMsg := ""
				if result.Error != nil {
					errMsg = result.Error.Error()
				}
				log.Warn(log.CatCommands, "command completed with error result",
					"command_id", cmd.ID(),
					"command_type", cmd.Type().String(),
					"trace_id", traceID,
					"duration", duration,
					"source", source,
					"error", errMsg,
				)
			} else {
				log.Debug(log.CatCommands, "command completed",
					"command_id", cmd.ID(),
					"command_type", cmd.Type().String(),
					"trace_id", traceID,
					"duration", duration,
					"source", source,
					"success", result != nil && result.Success,
				)
			}

			return result, err
		})
	}
}

// ===========================================================================
// Deduplication Middleware
// ===========================================================================

// DefaultDeduplicationTTL is the default time-to-live for deduplication cache entries.
const DefaultDeduplicationTTL = 5 * time.Second

// ErrDuplicateCommand is returned when a duplicate command is detected within the TTL window.
var ErrDuplicateCommand = types.ErrDuplicateCommand

// DeduplicationMiddlewareConfig configures the deduplication middleware.
type DeduplicationMiddlewareConfig struct {
	TTL             time.Duration
	CleanupInterval time.Duration // If 0, uses TTL/2
}

// DeduplicationMiddleware prevents duplicate commands from being processed
// within a configurable TTL window.
type DeduplicationMiddleware struct {
	cache      sync.Map // map[string]time.Time (hash -> expiry time)
	ttl        time.Duration
	cleanupCtx context.Context
	cancelFunc context.CancelFunc
	cleanupWg  sync.WaitGroup
	started    bool
	mu         sync.Mutex // protects started
}

// NewDeduplicationMiddleware creates a new deduplication middleware.
// It starts a background goroutine for cache cleanup.
func NewDeduplicationMiddleware(cfg DeduplicationMiddlewareConfig) *DeduplicationMiddleware {
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = DefaultDeduplicationTTL
	}

	cleanupInterval := cfg.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = ttl / 2
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &DeduplicationMiddleware{
		ttl:        ttl,
		cleanupCtx: ctx,
		cancelFunc: cancel,
	}

	m.startCleanup(cleanupInterval)
	return m
}

// startCleanup starts the background cleanup goroutine.
func (m *DeduplicationMiddleware) startCleanup(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return
	}
	m.started = true

	m.cleanupWg.Add(1)
	go func() {
		defer m.cleanupWg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-m.cleanupCtx.Done():
				return
			case <-ticker.C:
				m.cleanupExpired()
			}
		}
	}()
}

// cleanupExpired removes expired entries from the cache.
func (m *DeduplicationMiddleware) cleanupExpired() {
	now := time.Now()
	var cleaned int

	m.cache.Range(func(key, value any) bool {
		expiry := value.(time.Time)
		if now.After(expiry) {
			m.cache.Delete(key)
			cleaned++
		}
		return true
	})

	if cleaned > 0 {
		log.Debug(log.CatOrch, "deduplication cache cleanup",
			"entries_removed", cleaned,
		)
	}
}

// Stop stops the background cleanup goroutine.
func (m *DeduplicationMiddleware) Stop() {
	m.cancelFunc()
	m.cleanupWg.Wait()
}

// CacheSize returns the current number of entries in the cache.
// This is primarily for testing.
func (m *DeduplicationMiddleware) CacheSize() int {
	count := 0
	m.cache.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Middleware returns the middleware function.
func (m *DeduplicationMiddleware) Middleware() Middleware {
	return func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			// Compute content hash (excluding ID and timestamp)
			hash := m.computeContentHash(cmd)

			// Check if we've seen this hash recently
			now := time.Now()
			if existingExpiry, loaded := m.cache.Load(hash); loaded {
				expiry := existingExpiry.(time.Time)
				if now.Before(expiry) {
					// Duplicate detected within TTL
					log.Warn(log.CatOrch, "duplicate command rejected",
						"command_id", cmd.ID(),
						"command_type", cmd.Type().String(),
						"content_hash", hash[:16], // Log first 16 chars
					)
					return &command.CommandResult{
						Success: false,
						Error:   ErrDuplicateCommand,
					}, nil
				}
			}

			// Store the hash with expiry
			m.cache.Store(hash, now.Add(m.ttl))

			// Process the command
			return next.Handle(ctx, cmd)
		})
	}
}

// contentHasher is implemented by commands that want custom dedup hashing.
// Commands implement this to exclude transient fields like ID and timestamp.
type contentHasher interface {
	ContentHash() string
}

// computeContentHash computes a hash of the command content,
// excluding the ID and CreatedAt timestamp to allow detecting
// semantically duplicate commands.
func (m *DeduplicationMiddleware) computeContentHash(cmd command.Command) string {
	h := sha256.New()

	// Hash the command type
	h.Write([]byte(cmd.Type().String()))

	// If the command implements ContentHash, use that
	if hasher, ok := cmd.(contentHasher); ok {
		h.Write([]byte(hasher.ContentHash()))
		return hex.EncodeToString(h.Sum(nil))
	}

	// For commands without ContentHash, hash based on type-specific fields
	// This requires type-switching to exclude ID and timestamps
	switch c := cmd.(type) {
	case *command.SpawnProcessCommand:
		h.Write([]byte(c.ProcessID))
		h.Write([]byte(c.Role))
	case *command.RetireProcessCommand:
		h.Write([]byte(c.ProcessID))
		h.Write([]byte(c.Reason))
	case *command.ReplaceProcessCommand:
		h.Write([]byte(c.ProcessID))
		h.Write([]byte(c.Reason))
	case *command.AssignTaskCommand:
		h.Write([]byte(c.WorkerID))
		h.Write([]byte(c.TaskID))
		h.Write([]byte(c.Summary))
	case *command.AssignReviewCommand:
		h.Write([]byte(c.ReviewerID))
		h.Write([]byte(c.TaskID))
		h.Write([]byte(c.ImplementerID))
	case *command.ApproveCommitCommand:
		h.Write([]byte(c.ImplementerID))
		h.Write([]byte(c.TaskID))
	case *command.SendToProcessCommand:
		h.Write([]byte(c.ProcessID))
		h.Write([]byte(c.Content))
	case *command.BroadcastCommand:
		h.Write([]byte(c.Content))
		for _, w := range c.ExcludeWorkers {
			h.Write([]byte(w))
		}
	case *command.DeliverProcessQueuedCommand:
		h.Write([]byte(c.ProcessID))
	case *command.ReportCompleteCommand:
		h.Write([]byte(c.WorkerID))
		h.Write([]byte(c.Summary))
	case *command.ReportVerdictCommand:
		h.Write([]byte(c.WorkerID))
		h.Write([]byte(c.Verdict))
		h.Write([]byte(c.Comments))
	case *command.TransitionPhaseCommand:
		h.Write([]byte(c.WorkerID))
		h.Write([]byte(c.NewPhase))
	default:
		// For unknown command types, include priority as a distinguishing field
		// This is a fallback that should rarely be hit with proper type switches
		h.Write(fmt.Appendf(nil, "%d", cmd.Priority()))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// ===========================================================================
// Command Log Middleware
// ===========================================================================

// CommandLogMiddlewareConfig configures the command log middleware.
type CommandLogMiddlewareConfig struct {
	// EventBus is the pub/sub broker for publishing CommandLogEvents.
	// If nil, the middleware will be a no-op.
	EventBus EventPublisher
}

// EventPublisher is an interface for publishing events.
// This allows the middleware to be tested with a mock publisher.
// Note: This uses a string type for eventType to avoid coupling to pubsub package.
type EventPublisher interface {
	Publish(eventType string, payload any)
}

// NewCommandLogMiddleware creates a middleware that emits CommandLogEvent for each
// processed command. This provides visibility into command processing for the UI.
// If the eventBus is nil, the middleware is a no-op (graceful degradation).
func NewCommandLogMiddleware(cfg CommandLogMiddlewareConfig) Middleware {
	return func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			// If no event bus, just pass through
			if cfg.EventBus == nil {
				return next.Handle(ctx, cmd)
			}

			start := time.Now()

			// Execute the handler
			result, err := next.Handle(ctx, cmd)

			// Calculate duration
			duration := time.Since(start)

			// Determine success and error
			var success bool
			var cmdErr error

			if err != nil {
				// Handler returned an error
				success = false
				cmdErr = err
			} else if result != nil && !result.Success {
				// Handler returned a failure result
				success = false
				cmdErr = result.Error
			} else {
				// Success
				success = true
			}

			// Extract source if available
			var source command.CommandSource
			if hasSource, ok := cmd.(interface{ Source() command.CommandSource }); ok {
				source = hasSource.Source()
			}

			// Extract trace ID if available
			var traceID string
			if hasTraceID, ok := cmd.(interface{ TraceID() string }); ok {
				traceID = hasTraceID.TraceID()
			}

			// Emit the event
			event := CommandLogEvent{
				CommandID:   cmd.ID(),
				CommandType: cmd.Type(),
				Source:      source,
				Success:     success,
				Error:       cmdErr,
				Duration:    duration,
				Timestamp:   time.Now(),
				TraceID:     traceID,
			}
			cfg.EventBus.Publish("updated", event)

			return result, err
		})
	}
}

// ===========================================================================
// Timeout Middleware
// ===========================================================================

// DefaultTimeoutWarningThreshold is the default threshold for logging slow handler warnings.
const DefaultTimeoutWarningThreshold = 100 * time.Millisecond

// TimeoutMiddlewareConfig configures the timeout middleware.
type TimeoutMiddlewareConfig struct {
	WarningThreshold time.Duration
}

// NewTimeoutMiddleware creates a middleware that logs warnings when handlers
// exceed the configured threshold.
// IMPORTANT: This middleware does NOT abort slow handlers - doing so could
// leave the system in an inconsistent state. It only logs warnings for
// performance monitoring.
func NewTimeoutMiddleware(cfg TimeoutMiddlewareConfig) Middleware {
	threshold := cfg.WarningThreshold
	if threshold == 0 {
		threshold = DefaultTimeoutWarningThreshold
	}

	return func(next CommandHandler) CommandHandler {
		return HandlerFunc(func(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
			start := time.Now()

			// Execute the handler
			result, err := next.Handle(ctx, cmd)

			// Check if handler exceeded threshold
			duration := time.Since(start)
			if duration > threshold {
				// Extract trace ID if available
				traceID := ""
				if hasTraceID, ok := cmd.(interface{ TraceID() string }); ok {
					traceID = hasTraceID.TraceID()
				}

				log.Warn(log.CatOrch, "handler exceeded time threshold",
					"command_id", cmd.ID(),
					"command_type", cmd.Type().String(),
					"trace_id", traceID,
					"duration", duration,
					"threshold", threshold,
				)
			}

			return result, err
		})
	}
}
