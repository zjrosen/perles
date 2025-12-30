package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const metadataFilename = "metadata.json"

// Metadata is the JSON-serializable session information.
type Metadata struct {
	// SessionID is the unique session identifier (UUID).
	SessionID string `json:"session_id"`

	// StartTime is when the session was created.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the session ended (zero if still running).
	EndTime time.Time `json:"end_time,omitzero"`

	// Status is the current session state.
	Status Status `json:"status"`

	// WorkDir is the working directory where the session was started.
	WorkDir string `json:"work_dir"`

	// CoordinatorID is the coordinator's process identifier.
	CoordinatorID string `json:"coordinator_id,omitempty"`

	// Workers contains metadata for each spawned worker.
	Workers []WorkerMetadata `json:"workers"`

	// ClientType is the AI client type (e.g., "claude").
	ClientType string `json:"client_type"`

	// Model is the AI model used (e.g., "sonnet").
	Model string `json:"model,omitempty"`

	// TokenUsage aggregates token usage across the session.
	TokenUsage TokenUsageSummary `json:"token_usage,omitzero"`
}

// WorkerMetadata tracks individual worker lifecycle.
type WorkerMetadata struct {
	// ID is the worker identifier (e.g., "worker-1").
	ID string `json:"id"`

	// SpawnedAt is when the worker was created.
	SpawnedAt time.Time `json:"spawned_at"`

	// RetiredAt is when the worker was shut down (zero if still active).
	RetiredAt time.Time `json:"retired_at,omitzero"`

	// FinalPhase is the worker's last workflow phase before retirement.
	FinalPhase string `json:"final_phase,omitempty"`
}

// TokenUsageSummary aggregates token usage across the session.
type TokenUsageSummary struct {
	// TotalInputTokens is the total number of input tokens used.
	TotalInputTokens int `json:"total_input_tokens"`

	// TotalOutputTokens is the total number of output tokens used.
	TotalOutputTokens int `json:"total_output_tokens"`

	// TotalCostUSD is the estimated total cost in USD.
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// Save writes metadata to metadata.json in the given directory.
// It creates the directory if it doesn't exist.
func (m *Metadata) Save(dir string) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	// Write to file
	path := filepath.Join(dir, metadataFilename)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing metadata file: %w", err)
	}

	return nil
}

// Load reads metadata from metadata.json in the given directory.
func Load(dir string) (*Metadata, error) {
	path := filepath.Join(dir, metadataFilename)

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("metadata file not found: %w", err)
		}
		return nil, fmt.Errorf("reading metadata file: %w", err)
	}

	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshaling metadata: %w", err)
	}

	return &m, nil
}
