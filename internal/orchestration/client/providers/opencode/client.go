package opencode

import (
	"context"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func init() {
	client.RegisterClient(client.ClientOpenCode, func() client.HeadlessClient {
		return NewClient()
	})
}

// OpenCodeClient implements client.HeadlessClient for OpenCode CLI.
type OpenCodeClient struct{}

// NewClient creates a new OpenCodeClient.
func NewClient() *OpenCodeClient {
	return &OpenCodeClient{}
}

// Type returns the client type identifier.
func (c *OpenCodeClient) Type() client.ClientType {
	return client.ClientOpenCode
}

// Spawn creates and starts a headless OpenCode process.
// If cfg.SessionID is set, resumes an existing session.
// If cfg.SessionID is empty, creates a new session.
func (c *OpenCodeClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	opencodeCfg := configFromClient(cfg)
	if cfg.SessionID != "" {
		return Resume(ctx, cfg.SessionID, opencodeCfg)
	}
	return Spawn(ctx, opencodeCfg)
}

// Ensure OpenCodeClient implements client.HeadlessClient at compile time.
var _ client.HeadlessClient = (*OpenCodeClient)(nil)
