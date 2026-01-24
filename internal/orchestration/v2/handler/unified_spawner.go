// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains the UnifiedProcessSpawnerImpl that creates AI processes
// for the unified architecture.
package handler

import (
	"context"
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// SpawnOptions contains optional configuration for spawning a process.
type SpawnOptions struct {
	// AgentType specifies the worker specialization (e.g., implementer, reviewer, researcher).
	// Defaults to AgentTypeGeneric if not specified.
	AgentType roles.AgentType

	// WorkflowConfig contains optional workflow-specific prompt customizations.
	// If nil, the default prompts for the agent type are used.
	// If set, the compose functions use three-tier resolution:
	// base prompts → override (if set) → append (if set).
	WorkflowConfig *roles.WorkflowConfig

	// TODO we should eventually just refactor this out and combine with the WorkflowConfig
	// InitialPromptOverride overrides the initial prompt sent to the process.
	// Empty string means use the default prompt.
	InitialPromptOverride string

	// TODO we should eventually just refactor this out and combine with the WorkflowConfig
	// SystemPromptOverride overrides the system prompt for the process.
	// Empty string means use the default prompt.
	SystemPromptOverride string
}

// UnifiedProcessSpawnerImpl implements UnifiedProcessSpawner for spawning real AI processes.
// It creates process.Process instances that manage the AI event loop.
// Supports different clients for coordinator and workers.
type UnifiedProcessSpawnerImpl struct {
	coordinatorClient     client.HeadlessClient
	workerClient          client.HeadlessClient
	coordinatorExtensions map[string]any
	workerExtensions      map[string]any
	workDir               string
	port                  int
	submitter             process.CommandSubmitter
	eventBus              *pubsub.Broker[any]
	beadsDir              string
}

// UnifiedSpawnerConfig holds configuration for creating a UnifiedProcessSpawnerImpl.
type UnifiedSpawnerConfig struct {
	// CoordinatorClient is the AI client for spawning coordinators.
	CoordinatorClient client.HeadlessClient
	// WorkerClient is the AI client for spawning workers.
	// If nil, uses CoordinatorClient for workers as well.
	WorkerClient client.HeadlessClient
	// CoordinatorExtensions holds provider-specific config for coordinator.
	CoordinatorExtensions map[string]any
	// WorkerExtensions holds provider-specific config for workers.
	WorkerExtensions map[string]any
	WorkDir          string
	Port             int
	Submitter        process.CommandSubmitter
	EventBus         *pubsub.Broker[any]
	// BeadsDir is the path to the beads database directory.
	// When set, spawned processes receive BEADS_DIR environment variable.
	BeadsDir string
}

// NewUnifiedProcessSpawner creates a new UnifiedProcessSpawnerImpl.
func NewUnifiedProcessSpawner(cfg UnifiedSpawnerConfig) *UnifiedProcessSpawnerImpl {
	// Fall back to coordinator client if worker client not set
	workerClient := cfg.WorkerClient
	if workerClient == nil {
		workerClient = cfg.CoordinatorClient
	}
	workerExtensions := cfg.WorkerExtensions
	if workerExtensions == nil {
		workerExtensions = cfg.CoordinatorExtensions
	}

	return &UnifiedProcessSpawnerImpl{
		coordinatorClient:     cfg.CoordinatorClient,
		workerClient:          workerClient,
		coordinatorExtensions: cfg.CoordinatorExtensions,
		workerExtensions:      workerExtensions,
		workDir:               cfg.WorkDir,
		port:                  cfg.Port,
		submitter:             cfg.Submitter,
		eventBus:              cfg.EventBus,
		beadsDir:              cfg.BeadsDir,
	}
}

// SpawnProcess creates and starts a new AI process.
// The opts parameter provides optional configuration like AgentType for worker specialization.
// Returns the created process.Process instance.
// Uses different clients for coordinator vs workers based on configuration.
func (s *UnifiedProcessSpawnerImpl) SpawnProcess(ctx context.Context, id string, role repository.ProcessRole, opts SpawnOptions) (*process.Process, error) {
	// Select client and extensions based on role
	var aiClient client.HeadlessClient
	var extensions map[string]any

	if role == repository.RoleCoordinator {
		aiClient = s.coordinatorClient
		extensions = s.coordinatorExtensions
	} else {
		aiClient = s.workerClient
		extensions = s.workerExtensions
	}

	if aiClient == nil {
		return nil, fmt.Errorf("client is nil for role %s", role)
	}

	// Generate appropriate config based on role
	var cfg client.Config
	if role == repository.RoleCoordinator {
		// Coordinator uses coordinator system prompt and MCP config
		mcpConfig, err := s.generateCoordinatorMCPConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to generate coordinator MCP config: %w", err)
		}

		// Apply system prompt override or use default
		var systemPrompt string
		if opts.SystemPromptOverride != "" {
			systemPrompt = opts.SystemPromptOverride
		}
		if opts.WorkflowConfig != nil && opts.WorkflowConfig.SystemPromptOverride != "" {
			systemPrompt = opts.WorkflowConfig.SystemPromptOverride
		}
		if systemPrompt == "" {
			systemPrompt, err = prompt.BuildCoordinatorSystemPrompt()
			if err != nil {
				return nil, fmt.Errorf("failed to build coordinator system prompt: %w", err)
			}
		}

		// Apply initial prompt override or use default
		var initialPrompt string
		if opts.InitialPromptOverride != "" {
			initialPrompt = opts.InitialPromptOverride
		}
		if opts.WorkflowConfig != nil && opts.WorkflowConfig.InitialPromptOverride != "" {
			initialPrompt = opts.WorkflowConfig.InitialPromptOverride
		}
		if initialPrompt == "" {
			initialPrompt, err = prompt.BuildCoordinatorInitialPrompt()
			if err != nil {
				return nil, fmt.Errorf("failed to build coordinator initial prompt: %w", err)
			}
		}

		cfg = client.Config{
			WorkDir:         s.workDir,
			BeadsDir:        s.beadsDir,
			SystemPrompt:    systemPrompt,
			Prompt:          initialPrompt,
			MCPConfig:       mcpConfig,
			SkipPermissions: true,
			DisallowedTools: []string{"AskUserQuestion"},
			Extensions:      extensions,
		}
	} else {
		// Worker uses role-specific prompts based on AgentType
		mcpConfig, err := s.generateWorkerMCPConfig(id)
		if err != nil {
			return nil, fmt.Errorf("failed to generate MCP config: %w", err)
		}

		// Compose prompts using three-tier resolution:
		// 1. Base prompts from roles registry (based on AgentType)
		// 2. Workflow override (if WorkflowConfig.SystemPromptOverride is set)
		// 3. Workflow append (if WorkflowConfig.SystemPromptAppend is set)
		systemPrompt := roles.ComposeSystemPrompt(id, opts.AgentType, opts.WorkflowConfig)
		initialPrompt := roles.ComposeInitialPrompt(id, opts.AgentType, opts.WorkflowConfig)

		cfg = client.Config{
			WorkDir:         s.workDir,
			BeadsDir:        s.beadsDir,
			Prompt:          initialPrompt,
			SystemPrompt:    systemPrompt,
			MCPConfig:       mcpConfig,
			SkipPermissions: true,
			DisallowedTools: []string{"AskUserQuestion"},
			Extensions:      extensions,
		}
	}

	// Spawn the underlying AI process
	headlessProc, err := aiClient.Spawn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn AI process: %w", err)
	}

	// Create process.Process wrapper that manages event loop
	proc := process.New(id, role, headlessProc, s.submitter, s.eventBus)

	// Start the event loop
	proc.Start()

	return proc, nil
}

// generateCoordinatorMCPConfig returns the appropriate MCP config for the coordinator.
func (s *UnifiedProcessSpawnerImpl) generateCoordinatorMCPConfig() (string, error) {
	if s.coordinatorClient == nil {
		return mcp.GenerateCoordinatorConfigHTTP(s.port)
	}
	switch s.coordinatorClient.Type() {
	case client.ClientAmp:
		return mcp.GenerateCoordinatorConfigAmp(s.port)
	case client.ClientCodex:
		return mcp.GenerateCoordinatorConfigCodex(s.port), nil
	case client.ClientGemini:
		return mcp.GenerateCoordinatorConfigGemini(s.port)
	case client.ClientOpenCode:
		return mcp.GenerateCoordinatorConfigOpenCode(s.port)
	default:
		return mcp.GenerateCoordinatorConfigHTTP(s.port)
	}
}

// generateWorkerMCPConfig returns the appropriate MCP config format for workers based on client type.
func (s *UnifiedProcessSpawnerImpl) generateWorkerMCPConfig(processID string) (string, error) {
	if s.workerClient == nil {
		return mcp.GenerateWorkerConfigHTTP(s.port, processID)
	}
	switch s.workerClient.Type() {
	case client.ClientAmp:
		return mcp.GenerateWorkerConfigAmp(s.port, processID)
	case client.ClientCodex:
		return mcp.GenerateWorkerConfigCodex(s.port, processID), nil
	case client.ClientGemini:
		return mcp.GenerateWorkerConfigGemini(s.port, processID)
	case client.ClientOpenCode:
		return mcp.GenerateWorkerConfigOpenCode(s.port, processID)
	default:
		return mcp.GenerateWorkerConfigHTTP(s.port, processID)
	}
}
