package mock

import (
	"context"

	"perles/internal/orchestration/client"
)

// Client is a mock implementation of client.HeadlessClient for testing.
// It allows configuring spawn behavior via function fields.
type Client struct {
	// SpawnFunc is called when Spawn is invoked.
	// If nil, a new Process is returned.
	SpawnFunc func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error)

	// spawnCount tracks how many times Spawn was called
	spawnCount int
	// resumeCount tracks how many times Spawn was called with a SessionID (resume)
	resumeCount int
}

// NewClient creates a new mock client with default behavior.
// By default, Spawn returns new Process instances.
func NewClient() *Client {
	return &Client{}
}

// Type returns the client type identifier.
func (c *Client) Type() client.ClientType {
	return client.ClientMock
}

// Spawn creates a new mock process or resumes an existing one.
// If cfg.SessionID is set, this counts as a resume operation.
// If SpawnFunc is set, it delegates to that function.
// Otherwise, it returns a new Process.
func (c *Client) Spawn(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
	c.spawnCount++
	if cfg.SessionID != "" {
		c.resumeCount++
	}
	if c.SpawnFunc != nil {
		return c.SpawnFunc(ctx, cfg)
	}
	proc := NewProcess()
	if cfg.SessionID != "" {
		proc.sessionID = cfg.SessionID
	}
	return proc, nil
}

// SpawnCount returns how many times Spawn was called.
func (c *Client) SpawnCount() int {
	return c.spawnCount
}

// ResumeCount returns how many times Spawn was called with a SessionID (resume).
func (c *Client) ResumeCount() int {
	return c.resumeCount
}

// Reset clears the call counters.
func (c *Client) Reset() {
	c.spawnCount = 0
	c.resumeCount = 0
}

// init registers the mock client with the client registry.
func init() {
	client.RegisterClient(client.ClientMock, func() client.HeadlessClient {
		return NewClient()
	})
}
