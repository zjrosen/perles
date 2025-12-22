package coordinator

import (
	"fmt"
	"strings"
	"time"

	"perles/internal/log"
	"perles/internal/orchestration/claude"
	"perles/internal/orchestration/client"
	"perles/internal/orchestration/events"
	"perles/internal/orchestration/mcp"
	"perles/internal/orchestration/metrics"
)

// generateMCPConfig returns the appropriate MCP config format based on client type.
func (c *Coordinator) generateMCPConfig() (string, error) {
	switch c.client.Type() {
	case client.ClientAmp:
		return mcp.GenerateCoordinatorConfigAmp(8765)
	default:
		return mcp.GenerateCoordinatorConfig(c.workDir)
	}
}

// Start initializes and spawns the coordinator Claude session.
// This must be called before SendUserMessage.
func (c *Coordinator) Start() error {
	c.mu.Lock()

	if c.Status() != StatusPending {
		c.mu.Unlock()
		return fmt.Errorf("coordinator already started (status: %s)", c.Status())
	}

	c.setStatus(StatusStarting)
	log.Debug(logCat, "Starting coordinator")

	// Generate MCP config for coordinator tools
	mcpConfig, err := c.generateMCPConfig()
	if err != nil {
		c.setStatus(StatusFailed)
		c.mu.Unlock()
		return fmt.Errorf("generating MCP config: %w", err)
	}

	// Build system prompt with epic context
	systemPrompt, err := c.buildSystemPrompt()
	if err != nil {
		c.setStatus(StatusFailed)
		c.mu.Unlock()
		return fmt.Errorf("building system prompt: %w", err)
	}

	// Spawn the coordinator AI session
	cfg := client.Config{
		WorkDir:         c.workDir,
		Prompt:          systemPrompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,                        // Autonomous execution
		DisallowedTools: []string{"AskUserQuestion"}, // Prevent hanging
		Extensions: map[string]any{
			client.ExtClaudeModel: c.model,
		},
	}

	process, err := c.client.Spawn(c.ctx, cfg)
	if err != nil {
		c.setStatus(StatusFailed)
		c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
			Error: fmt.Errorf("spawning coordinator: %w", err),
		})
		c.mu.Unlock()
		return fmt.Errorf("spawning coordinator: %w", err)
	}

	c.process = process
	c.sessionID = process.SessionRef()
	log.Debug(logCat, "Coordinator started", "sessionID", c.sessionID)

	// Start event processing
	c.wg.Add(1)
	go c.processEvents()

	// Note: Pool events are published directly by pool.Broker()
	// Subscribers (TUI) subscribe via Coordinator.Workers() which delegates to pool.Broker()

	c.setStatus(StatusRunning)

	// Coordinator is now working (processing initial prompt)
	c.emitCoordinatorEvent(events.CoordinatorWorking, events.CoordinatorEvent{})

	// Release lock before I/O operations
	c.mu.Unlock()

	// Note: Workers are spawned by the coordinator agent via spawn_worker MCP tool
	// The system prompt instructs it to spawn 4 workers at startup

	return nil
}

// SendUserMessage forwards a message from the user to the coordinator.
// Returns an error if the coordinator is not running.
func (c *Coordinator) SendUserMessage(content string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check status and process under lock to prevent race conditions
	if c.Status() != StatusRunning {
		return fmt.Errorf("coordinator not running (status: %s)", c.Status())
	}

	if c.process == nil {
		return fmt.Errorf("coordinator process not available")
	}

	// Emit user message event for TUI
	c.emitCoordinatorEvent(events.CoordinatorChat, events.CoordinatorEvent{
		Role:    "user",
		Content: content,
	})

	// Coordinator is now working (processing user message)
	c.emitCoordinatorEvent(events.CoordinatorWorking, events.CoordinatorEvent{})

	// Resume the session with the user's message
	log.Debug(logCat, "Sending user message to coordinator", "content", content)

	// Generate MCP config for coordinator tools
	mcpConfig, err := c.generateMCPConfig()
	if err != nil {
		c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
			Error: fmt.Errorf("generating MCP config: %w", err),
		})
		return fmt.Errorf("generating MCP config: %w", err)
	}

	// Resume the session with the new message
	// The current process has completed, so we spawn a new one with resume
	cfg := client.Config{
		WorkDir:         c.workDir,
		Prompt:          content,
		SessionID:       c.sessionID,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions: map[string]any{
			client.ExtClaudeModel: c.model,
		},
	}

	newProcess, err := c.client.Spawn(c.ctx, cfg)
	if err != nil {
		c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
			Error: fmt.Errorf("resuming coordinator: %w", err),
		})
		return fmt.Errorf("resuming coordinator: %w", err)
	}

	c.process = newProcess

	// Start processing events from the new process
	c.wg.Add(1)
	go c.processEvents()

	return nil
}

// Pause pauses the coordinator workflow.
// Workers continue their current tasks but no new tasks are assigned.
func (c *Coordinator) Pause() error {
	if c.Status() != StatusRunning {
		return fmt.Errorf("coordinator not running (status: %s)", c.Status())
	}

	log.Debug(logCat, "Pausing coordinator")
	c.setStatus(StatusPaused)
	return nil
}

// Resume resumes a paused coordinator.
func (c *Coordinator) Resume() error {
	if c.Status() != StatusPaused {
		return fmt.Errorf("coordinator not paused (status: %s)", c.Status())
	}

	log.Debug(logCat, "Resuming coordinator")
	c.setStatus(StatusRunning)
	return nil
}

// Replace performs a hot swap of the coordinator Claude process with a FRESH session.
// This is used when the coordinator reaches context limits - it creates a new session
// with fresh context while preserving external state (workers, message log, tasks).
// The events channel, worker pool, and MCP server remain stable throughout.
func (c *Coordinator) Replace() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Must be running to replace
	if c.Status() != StatusRunning {
		return fmt.Errorf("coordinator not running (status: %s)", c.Status())
	}

	if c.process == nil {
		return fmt.Errorf("coordinator process not available")
	}

	log.Debug(logCat, "Replacing coordinator process with fresh session", "oldSessionID", c.sessionID)

	// Cancel the old process (but NOT c.ctx - the coordinator stays alive)
	// The old processEvents goroutine will exit when its channel closes
	oldProcess := c.process
	_ = oldProcess.Cancel() // Error ignored - we're replacing the process anyway

	// Generate MCP config for coordinator tools
	mcpConfig, err := c.generateMCPConfig()
	if err != nil {
		c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
			Error: fmt.Errorf("generating MCP config for replacement: %w", err),
		})
		return fmt.Errorf("generating MCP config: %w", err)
	}

	// Build comprehensive replacement prompt with context
	replacePrompt := c.buildReplacePrompt()

	// Spawn new process with FRESH session (no SessionID = new context window)
	cfg := client.Config{
		WorkDir:         c.workDir,
		Prompt:          replacePrompt,
		MCPConfig:       mcpConfig,
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions: map[string]any{
			client.ExtClaudeModel: c.model,
		},
		// NO SessionID - creates fresh session with empty context window
	}

	newProcess, err := c.client.Spawn(c.ctx, cfg)
	if err != nil {
		c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
			Error: fmt.Errorf("spawning replacement coordinator: %w", err),
		})
		return fmt.Errorf("spawning replacement coordinator: %w", err)
	}

	// Update process reference
	// Note: c.sessionID will be updated when processEvents receives the init event
	c.process = newProcess

	// Start processing events from the new process
	c.wg.Add(1)
	go c.processEvents()

	// Emit event for TUI notification
	c.emitCoordinatorEvent(events.CoordinatorChat, events.CoordinatorEvent{
		Role:    "system",
		Content: "Coordinator replaced with fresh context window",
	})

	// Coordinator is working (processing the refresh prompt)
	c.emitCoordinatorEvent(events.CoordinatorWorking, events.CoordinatorEvent{})

	log.Debug(logCat, "Coordinator process replaced with fresh session")
	return nil
}

// buildReplacePrompt creates a comprehensive prompt for a replacement coordinator.
// Since the new session has fresh context, we need to provide enough information
// for the coordinator to understand the current state and continue orchestrating.
func (c *Coordinator) buildReplacePrompt() string {
	var prompt strings.Builder

	prompt.WriteString("[CONTEXT REFRESH - NEW SESSION]\n\n")
	prompt.WriteString("Your context window was approaching limits, so you've been replaced with a fresh session.\n")
	prompt.WriteString("Your workers are still running and all external state is preserved.\n\n")

	prompt.WriteString("IMMEDIATE ACTIONS REQUIRED:\n")
	prompt.WriteString("1. Run `list_workers` to see current worker status and assignments\n")
	prompt.WriteString("2. Run `read_message_log` to see recent activity and any pending messages\n")

	prompt.WriteString("The worker pool, MCP server, and message log are all still active.\n")
	prompt.WriteString("You have full access to all coordinator tools.\n")

	return prompt.String()
}

// Cancel stops the coordinator and all workers.
func (c *Coordinator) Cancel() error {
	return c.stop()
}

// stop is the internal implementation of Cancel and Complete.
func (c *Coordinator) stop() error {
	c.mu.Lock()

	// Check if already stopped or stopping
	status := c.Status()
	if status == StatusStopped || status == StatusStopping {
		c.mu.Unlock()
		return nil
	}

	log.Debug(logCat, "Stopping coordinator")
	c.setStatus(StatusStopping)

	// Cancel context (stops Claude process)
	c.cancel()

	// Release lock while waiting for goroutines (avoids deadlock with processEvents)
	c.mu.Unlock()
	c.wg.Wait()
	c.mu.Lock()

	// Set final status BEFORE closing broker
	c.setStatus(StatusStopped)

	// Close coordinator broker (safe - brokers are idempotent)
	// Worker events are published by pool directly, pool handles its own broker
	c.broker.Close()

	c.mu.Unlock()

	return nil
}

// Wait blocks until the coordinator stops.
func (c *Coordinator) Wait() error {
	c.wg.Wait()
	return nil
}

// processEvents reads events from the AI process and emits them.
func (c *Coordinator) processEvents() {
	defer c.wg.Done()

	c.mu.RLock()
	process := c.process
	c.mu.RUnlock()

	if process == nil {
		return
	}

	aiEvents := process.Events()
	for {
		select {
		case <-c.ctx.Done():
			log.Debug(logCat, "processEvents exiting - context cancelled")
			return
		case event, ok := <-aiEvents:
			if !ok {
				log.Debug(logCat, "processEvents exiting - channel closed")
				return
			}

			// Convert Claude events to coordinator events
			switch {
			case event.IsInit():
				c.mu.Lock()
				c.sessionID = event.SessionID
				c.mu.Unlock()
				log.Debug(logCat, "Coordinator session initialized", "sessionID", event.SessionID)

			case event.IsAssistant() && event.Message != nil:
				// Emit assistant text as chat message
				text := event.Message.GetText()
				if text != "" {
					c.emitCoordinatorEvent(events.CoordinatorChat, events.CoordinatorEvent{
						Role:    "coordinator",
						Content: text,
					})
				}

				// Also emit tool calls for visibility
				for _, block := range event.Message.Content {
					if block.Type == "tool_use" && block.Name != "" {
						toolMsg := claude.FormatToolDisplay(&block)
						c.emitCoordinatorEvent(events.CoordinatorChat, events.CoordinatorEvent{
							Role:    "coordinator",
							Content: toolMsg,
						})
					}
				}

			case event.IsResult():
				// Handle result events - may be success or error (e.g., "Prompt is too long")
				log.Debug(logCat, "Coordinator result event",
					"hasUsage", event.Usage != nil,
					"isError", event.IsErrorResult,
					"result", event.Result)

				// Check for error results first (e.g., context window exceeded)
				if event.IsErrorResult {
					errMsg := event.GetErrorMessage()
					log.Debug(logCat, "Coordinator result error", "message", errMsg)
					// Show error as a chat message so user sees it in the coordinator pane
					c.emitCoordinatorEvent(events.CoordinatorChat, events.CoordinatorEvent{
						Role:    "coordinator",
						Content: "⚠️ Error: " + errMsg,
					})
					// Don't emit ready - coordinator is in error state
					continue
				}

				// Emit token usage from result event (has per-turn usage)
				if event.Usage != nil {
					// Build comprehensive TokenMetrics from event usage
					m := &metrics.TokenMetrics{
						InputTokens:              event.Usage.InputTokens,
						OutputTokens:             event.Usage.OutputTokens,
						CacheReadInputTokens:     event.Usage.CacheReadInputTokens,
						CacheCreationInputTokens: event.Usage.CacheCreationInputTokens,
						ContextTokens:            event.GetContextTokens(),
						ContextWindow:            c.getContextWindow(event),
						TurnCostUSD:              event.TotalCostUSD,
						LastUpdatedAt:            time.Now(),
					}

					// Accumulate total cost across turns
					c.mu.Lock()
					c.cumulativeMetrics.TotalCostUSD += m.TurnCostUSD
					m.TotalCostUSD = c.cumulativeMetrics.TotalCostUSD
					c.mu.Unlock()

					log.Debug(logCat, "Coordinator token usage from result",
						"input", event.Usage.InputTokens,
						"output", event.Usage.OutputTokens,
						"cache_read", event.Usage.CacheReadInputTokens,
						"cache_creation", event.Usage.CacheCreationInputTokens,
						"context", m.ContextTokens,
						"turnCost", m.TurnCostUSD,
						"totalCost", m.TotalCostUSD)

					if m.ContextTokens > 0 {
						c.emitCoordinatorEvent(events.CoordinatorTokenUsage, events.CoordinatorEvent{
							Metrics: m,
						})
						log.Debug(logCat, "Emitted CoordinatorTokenUsage", "contextTokens", m.ContextTokens)
					}
				}

				// Process completed successfully - coordinator is now ready for input
				log.Debug(logCat, "Coordinator process completed",
					"cost", event.TotalCostUSD,
					"durationMs", event.DurationMs)
				c.emitCoordinatorEvent(events.CoordinatorReady, events.CoordinatorEvent{})

			case event.IsToolResult():
				// Tool results may indicate worker actions
				toolName := ""
				if event.Tool != nil {
					toolName = event.Tool.Name
				}
				log.Debug(logCat, "Coordinator tool result", "tool", toolName)

			case event.IsError():
				// Handle explicit error events (type: "error")
				errMsg := event.GetErrorMessage()
				log.Debug(logCat, "Coordinator error event", "message", errMsg)
				c.emitCoordinatorEvent(events.CoordinatorError, events.CoordinatorEvent{
					Error: fmt.Errorf("%s", errMsg),
				})
			}
		}
	}
}

// getContextWindow returns the context window size from the event or a default.
func (c *Coordinator) getContextWindow(event client.OutputEvent) int {
	// Try to get from ModelUsage first (has ContextWindow field)
	for _, usage := range event.ModelUsage {
		if usage.ContextWindow > 0 {
			return usage.ContextWindow
		}
	}
	// Default context window for Claude models
	return 200000
}
