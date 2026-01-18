package gemini

import (
	"context"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func init() {
	client.RegisterClient(client.ClientGemini, func() client.HeadlessClient {
		return NewClient()
	})
}

// GeminiClient implements client.HeadlessClient for Gemini CLI.
type GeminiClient struct{}

// NewClient creates a new GeminiClient.
func NewClient() *GeminiClient {
	return &GeminiClient{}
}

// Type returns the client type identifier.
func (c *GeminiClient) Type() client.ClientType {
	return client.ClientGemini
}

// Spawn creates and starts a headless Gemini process.
// If cfg.SessionID is set, resumes an existing session.
// If cfg.SessionID is empty, creates a new session.
func (c *GeminiClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	geminiCfg := configFromClient(cfg)
	if cfg.SessionID != "" {
		return Resume(ctx, cfg.SessionID, geminiCfg)
	}
	return Spawn(ctx, geminiCfg)
}

// Ensure GeminiClient implements client.HeadlessClient at compile time.
var _ client.HeadlessClient = (*GeminiClient)(nil)
