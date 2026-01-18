package claude

import (
	"context"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func init() {
	client.RegisterClient(client.ClientClaude, func() client.HeadlessClient {
		return NewClient()
	})
}

// ClaudeClient implements client.HeadlessClient for Claude Code CLI.
type ClaudeClient struct{}

// NewClient creates a new ClaudeClient.
func NewClient() *ClaudeClient {
	return &ClaudeClient{}
}

// Type returns the client type identifier.
func (c *ClaudeClient) Type() client.ClientType {
	return client.ClientClaude
}

// Spawn creates and starts a headless Claude process.
// If cfg.SessionID is set, resumes an existing session.
// If cfg.SessionID is empty, creates a new session.
func (c *ClaudeClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	claudeCfg := configFromClient(cfg)
	if cfg.SessionID != "" {
		return ResumeWithConfig(ctx, cfg.SessionID, claudeCfg)
	}
	return Spawn(ctx, claudeCfg)
}

// configFromClient converts a client.Config to a claude.Config.
func configFromClient(cfg client.Config) Config {
	return Config{
		WorkDir:            cfg.WorkDir,
		Prompt:             cfg.Prompt,
		SessionID:          cfg.SessionID,
		Model:              cfg.ClaudeModel(),
		AppendSystemPrompt: cfg.SystemPrompt,
		AllowedTools:       cfg.AllowedTools,
		DisallowedTools:    cfg.DisallowedTools,
		SkipPermissions:    cfg.SkipPermissions,
		Timeout:            cfg.Timeout,
		MCPConfig:          cfg.MCPConfig,
	}
}

// Ensure ClaudeClient implements client.HeadlessClient at compile time.
var _ client.HeadlessClient = (*ClaudeClient)(nil)
