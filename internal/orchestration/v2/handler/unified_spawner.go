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
}

// UnifiedProcessSpawnerImpl implements UnifiedProcessSpawner for spawning real AI processes.
// It creates process.Process instances that manage the AI event loop.
type UnifiedProcessSpawnerImpl struct {
	client     client.HeadlessClient
	workDir    string
	port       int
	extensions map[string]any
	submitter  process.CommandSubmitter
	eventBus   *pubsub.Broker[any]
}

// UnifiedSpawnerConfig holds configuration for creating a UnifiedProcessSpawnerImpl.
type UnifiedSpawnerConfig struct {
	Client     client.HeadlessClient
	WorkDir    string
	Port       int
	Extensions map[string]any
	Submitter  process.CommandSubmitter
	EventBus   *pubsub.Broker[any]
}

// NewUnifiedProcessSpawner creates a new UnifiedProcessSpawnerImpl.
func NewUnifiedProcessSpawner(cfg UnifiedSpawnerConfig) *UnifiedProcessSpawnerImpl {
	return &UnifiedProcessSpawnerImpl{
		client:     cfg.Client,
		workDir:    cfg.WorkDir,
		port:       cfg.Port,
		extensions: cfg.Extensions,
		submitter:  cfg.Submitter,
		eventBus:   cfg.EventBus,
	}
}

// SpawnProcess creates and starts a new AI process.
// The opts parameter provides optional configuration like AgentType for worker specialization.
// Returns the created process.Process instance.
func (s *UnifiedProcessSpawnerImpl) SpawnProcess(ctx context.Context, id string, role repository.ProcessRole, opts SpawnOptions) (*process.Process, error) {
	if s.client == nil {
		return nil, fmt.Errorf("client is nil")
	}

	// Generate appropriate config based on role
	var cfg client.Config
	if role == repository.RoleCoordinator {
		// Coordinator uses coordinator system prompt and MCP config
		mcpConfig, err := s.generateCoordinatorMCPConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to generate coordinator MCP config: %w", err)
		}
		systemPrompt, err := prompt.BuildCoordinatorSystemPrompt()
		if err != nil {
			return nil, fmt.Errorf("failed to build coordinator system prompt: %w", err)
		}
		initialPrompt, err := prompt.BuildCoordinatorInitialPrompt()
		if err != nil {
			return nil, fmt.Errorf("failed to build coordinator initial prompt: %w", err)
		}
		cfg = client.Config{
			WorkDir:         s.workDir,
			SystemPrompt:    systemPrompt,
			Prompt:          initialPrompt,
			MCPConfig:       mcpConfig,
			SkipPermissions: true,
			DisallowedTools: []string{"AskUserQuestion"},
			Extensions:      s.extensions,
		}
	} else {
		// Worker uses role-specific prompts based on AgentType
		mcpConfig, err := s.generateMCPConfig(id)
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
			Prompt:          initialPrompt,
			SystemPrompt:    systemPrompt,
			MCPConfig:       mcpConfig,
			SkipPermissions: true,
			DisallowedTools: []string{"AskUserQuestion"},
			Extensions:      s.extensions,
		}
	}

	// Spawn the underlying AI process
	headlessProc, err := s.client.Spawn(ctx, cfg)
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
	if s.client == nil {
		return mcp.GenerateCoordinatorConfigHTTP(s.port)
	}
	switch s.client.Type() {
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

// generateMCPConfig returns the appropriate MCP config format for workers based on client type.
func (s *UnifiedProcessSpawnerImpl) generateMCPConfig(processID string) (string, error) {
	if s.client == nil {
		return mcp.GenerateWorkerConfigHTTP(s.port, processID)
	}
	switch s.client.Type() {
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
