package amp

import (
	"context"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func init() {
	client.RegisterClient(client.ClientAmp, func() client.HeadlessClient {
		return NewClient()
	})
}

// AmpClient implements client.HeadlessClient for Amp CLI.
type AmpClient struct{}

// NewClient creates a new AmpClient.
func NewClient() *AmpClient {
	return &AmpClient{}
}

// Type returns the client type identifier.
func (c *AmpClient) Type() client.ClientType {
	return client.ClientAmp
}

// Spawn creates and starts a headless Amp process.
// If cfg.SessionID is set, resumes an existing thread.
// If cfg.SessionID is empty, creates a new thread.
func (c *AmpClient) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	ampCfg := configFromClient(cfg)
	if cfg.SessionID != "" {
		return Resume(ctx, cfg.SessionID, ampCfg)
	}
	return Spawn(ctx, ampCfg)
}

// Ensure AmpClient implements client.HeadlessClient at compile time.
var _ client.HeadlessClient = (*AmpClient)(nil)
