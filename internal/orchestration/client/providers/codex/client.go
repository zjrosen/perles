package codex

import (
	"context"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func init() {
	client.RegisterClient(client.ClientCodex, func() client.HeadlessClient {
		return NewClient()
	})
}

// CodexClient implements client.HeadlessClient for Codex CLI.
type CodexClient struct{}

// NewClient creates a new CodexClient.
func NewClient() *CodexClient {
	return &CodexClient{}
}

// Type returns the client type identifier.
func (c *CodexClient) Type() client.ClientType {
	return client.ClientCodex
}

// Spawn creates and starts a headless Codex process.
// If cfg.SessionID is set, resumes an existing session.
// If cfg.SessionID is empty, creates a new session.
func (c *CodexClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	codexCfg := configFromClient(cfg)
	if cfg.SessionID != "" {
		return Resume(ctx, cfg.SessionID, codexCfg)
	}
	return Spawn(ctx, codexCfg)
}

// Ensure CodexClient implements client.HeadlessClient at compile time.
var _ client.HeadlessClient = (*CodexClient)(nil)
