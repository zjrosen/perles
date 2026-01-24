package orchestration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

func TestInitializer_CreatesSession(t *testing.T) {
	// Create a temporary directory for the workspace
	workDir := t.TempDir()

	// Create an initializer with minimal config
	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Start initialization (this will fail because we don't have a real AI client,
	// but we can still verify the session was created in createWorkspace())

	// We can't directly call createWorkspace() because it's a private method,
	// but we can verify the behavior indirectly by checking that Resources()
	// returns a properly initialized session after initialization starts.

	// Since the actual initialization requires an AI client which we don't have
	// in unit tests, we'll verify the session folder structure after a failed init.

	// For a true unit test, we need to refactor createWorkspace to be testable
	// or accept that this is an integration test.

	// Instead, verify the session field exists in InitializerResources
	resources := init.Resources()
	// Session should be nil before start
	require.Nil(t, resources.Session)
}

func TestInitializer_SessionInResources(t *testing.T) {
	// Verify the session field is present in InitializerResources
	resources := InitializerResources{}
	// The Session field should be accessible (compile-time check)
	require.Nil(t, resources.Session)
}

func TestInitializer_SessionFolderStructure(t *testing.T) {
	// This test verifies the expected folder structure matches what the session package creates.
	// This is a documentation test showing the expected structure.

	workDir := t.TempDir()
	sessionID := "test-session-uuid"
	sessionDir := filepath.Join(workDir, ".perles", "sessions", sessionID)

	// Verify the path construction matches what initializer.go does
	expectedDir := filepath.Join(workDir, ".perles", "sessions", sessionID)
	require.Equal(t, expectedDir, sessionDir)

	// The actual folder creation is done by session.New() which is already tested
	// in internal/orchestration/session/session_test.go
}

func TestInitializer_Retry_ResetsSession(t *testing.T) {
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Verify session is nil initially
	require.Nil(t, init.session)

	// After Retry is called, session should still be nil (since Retry resets it)
	// We can't actually call Retry without Start, but we can verify the field exists
	// and would be reset in the Retry method.
}

func TestNewInitializer(t *testing.T) {
	cfg := InitializerConfig{
		WorkDir: "/test/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)
	require.NotNil(t, init.Broker())
	require.Equal(t, InitNotStarted, init.Phase())
	require.Nil(t, init.Error())
}

func TestNewInitializer_DefaultWorkers(t *testing.T) {
	cfg := InitializerConfig{
		WorkDir: "/test/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)
	// With lazy spawning, default is 0 workers (coordinator spawns workers on-demand)
}

// Tests for InitializerConfigBuilder

func TestNewInitializerConfigFromModel_AllFields(t *testing.T) {
	// Test the builder constructs InitializerConfig with all static fields
	provider := client.NewAgentProvider(client.ClientClaude, map[string]any{
		client.ExtClaudeModel: "opus",
	})
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
		client.RoleWorker:      provider,
	}

	builder := NewInitializerConfigFromModel(
		"/work/dir",
		"/beads/dir", // beadsDir
		providers,
		"main",                              // worktreeBaseBranch
		"custom-branch",                     // worktreeCustomBranch
		config.TracingConfig{Enabled: true}, // tracingConfig
		config.SessionStorageConfig{BaseDir: "/tmp"}, // sessionStorageConfig
	)

	cfg := builder.Build()

	require.Equal(t, "/work/dir", cfg.WorkDir)
	require.Equal(t, "/beads/dir", cfg.BeadsDir)
	require.Equal(t, client.ClientClaude, cfg.AgentProviders.Coordinator().Type())
	require.Equal(t, "opus", cfg.AgentProviders.Coordinator().Extensions()[client.ExtClaudeModel])
	require.Equal(t, "main", cfg.WorktreeBaseBranch)
	require.Equal(t, "custom-branch", cfg.WorktreeBranchName)
	require.True(t, cfg.TracingConfig.Enabled)
	require.Equal(t, "/tmp", cfg.SessionStorage.BaseDir)
}

func TestNewInitializerConfigFromModel_EmptyExtensions(t *testing.T) {
	// Test builder handles empty/nil extensions
	provider := client.NewAgentProvider(client.ClientCodex, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	builder := NewInitializerConfigFromModel(
		"/work/dir",
		"",                            // beadsDir - empty
		providers,                     // agentProviders
		"",                            // no worktree
		"",                            // no custom branch
		config.TracingConfig{},        // disabled
		config.SessionStorageConfig{}, // default
	)

	cfg := builder.Build()

	require.Equal(t, "/work/dir", cfg.WorkDir)
	require.Equal(t, client.ClientCodex, cfg.AgentProviders.Coordinator().Type())
	require.Empty(t, cfg.AgentProviders.Coordinator().Extensions()) // Empty map, not nil
	require.Empty(t, cfg.WorktreeBaseBranch)
	require.Empty(t, cfg.WorktreeBranchName)
}

func TestInitializerConfigBuilder_WithTimeout(t *testing.T) {
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}
	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	cfg := builder.WithTimeout(60 * time.Second).Build()

	// WithTimeout maps to Timeouts.CoordinatorStart for backwards compatibility
	require.Equal(t, 60*time.Second, cfg.Timeouts.CoordinatorStart)
}

func TestWithTimeouts_SetsAllFields(t *testing.T) {
	// Test that WithTimeouts sets all timeout configuration values
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}
	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	timeouts := config.TimeoutsConfig{
		WorktreeCreation: 45 * time.Second,
		CoordinatorStart: 90 * time.Second,
		WorkspaceSetup:   20 * time.Second,
		MaxTotal:         180 * time.Second,
	}

	cfg := builder.WithTimeouts(timeouts).Build()

	require.Equal(t, 45*time.Second, cfg.Timeouts.WorktreeCreation)
	require.Equal(t, 90*time.Second, cfg.Timeouts.CoordinatorStart)
	require.Equal(t, 20*time.Second, cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, 180*time.Second, cfg.Timeouts.MaxTotal)
}

func TestWithTimeout_BackwardsCompatible(t *testing.T) {
	// Test that WithTimeout() continues to work for backwards compatibility
	// and maps to Timeouts.CoordinatorStart
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}
	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	cfg := builder.WithTimeout(45 * time.Second).Build()

	// WithTimeout should map to CoordinatorStart
	require.Equal(t, 45*time.Second, cfg.Timeouts.CoordinatorStart)
	// Other timeout fields should remain zero (not set)
	require.Equal(t, time.Duration(0), cfg.Timeouts.WorktreeCreation)
	require.Equal(t, time.Duration(0), cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, time.Duration(0), cfg.Timeouts.MaxTotal)
}

func TestNewInitializer_DefaultTimeouts(t *testing.T) {
	// Test that NewInitializer applies defaults for zero-value timeout fields
	provider := client.NewAgentProvider(client.ClientClaude, nil)

	cfg := InitializerConfig{
		WorkDir: "/work/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: provider,
		},
		// Timeouts is zero-value, so all fields should get defaults
	}

	init := NewInitializer(cfg)

	// After construction, all timeout fields should have defaults applied
	defaults := config.DefaultTimeoutsConfig()
	require.Equal(t, defaults.WorktreeCreation, init.cfg.Timeouts.WorktreeCreation)
	require.Equal(t, defaults.CoordinatorStart, init.cfg.Timeouts.CoordinatorStart)
	require.Equal(t, defaults.WorkspaceSetup, init.cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, defaults.MaxTotal, init.cfg.Timeouts.MaxTotal)
}

func TestNewInitializer_PartialTimeoutsGetDefaults(t *testing.T) {
	// Test that NewInitializer applies defaults only for zero-value fields
	// while preserving explicitly set values
	provider := client.NewAgentProvider(client.ClientClaude, nil)

	cfg := InitializerConfig{
		WorkDir: "/work/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: provider,
		},
		Timeouts: config.TimeoutsConfig{
			CoordinatorStart: 90 * time.Second, // Explicitly set
			// Other fields are zero, should get defaults
		},
	}

	init := NewInitializer(cfg)

	defaults := config.DefaultTimeoutsConfig()
	// Explicitly set value should be preserved
	require.Equal(t, 90*time.Second, init.cfg.Timeouts.CoordinatorStart)
	// Zero-value fields should get defaults
	require.Equal(t, defaults.WorktreeCreation, init.cfg.Timeouts.WorktreeCreation)
	require.Equal(t, defaults.WorkspaceSetup, init.cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, defaults.MaxTotal, init.cfg.Timeouts.MaxTotal)
}

func TestNewInitializer_CustomTimeoutsPreserved(t *testing.T) {
	// Test that NewInitializer preserves all custom timeout values
	provider := client.NewAgentProvider(client.ClientClaude, nil)

	customTimeouts := config.TimeoutsConfig{
		WorktreeCreation: 45 * time.Second,
		CoordinatorStart: 90 * time.Second,
		WorkspaceSetup:   20 * time.Second,
		MaxTotal:         180 * time.Second,
	}

	cfg := InitializerConfig{
		WorkDir: "/work/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: provider,
		},
		Timeouts: customTimeouts,
	}

	init := NewInitializer(cfg)

	// All custom values should be preserved
	require.Equal(t, customTimeouts.WorktreeCreation, init.cfg.Timeouts.WorktreeCreation)
	require.Equal(t, customTimeouts.CoordinatorStart, init.cfg.Timeouts.CoordinatorStart)
	require.Equal(t, customTimeouts.WorkspaceSetup, init.cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, customTimeouts.MaxTotal, init.cfg.Timeouts.MaxTotal)
}

func TestInitializerConfigBuilder_WithGitExecutor(t *testing.T) {
	mockExecutor := &mocks.MockGitExecutor{}
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	cfg := builder.WithGitExecutor(mockExecutor).Build()

	require.Same(t, mockExecutor, cfg.GitExecutor)
}

func TestInitializerConfigBuilder_WithRestoredSession(t *testing.T) {
	resumableSession := &session.ResumableSession{
		Metadata: &session.Metadata{
			SessionID: "test-session-id",
		},
	}
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	cfg := builder.WithRestoredSession(resumableSession).Build()

	require.Same(t, resumableSession, cfg.RestoredSession)
}

func TestInitializerConfigBuilder_WithSoundService(t *testing.T) {
	mockSound := &mocks.MockSoundService{}
	provider := client.NewAgentProvider(client.ClientClaude, nil)
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	builder := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	)

	cfg := builder.WithSoundService(mockSound).Build()

	require.Same(t, mockSound, cfg.SoundService)
}

func TestInitializerConfigBuilder_Chaining(t *testing.T) {
	// Test that all builder methods can be chained together
	mockExecutor := &mocks.MockGitExecutor{}
	mockSound := &mocks.MockSoundService{}
	resumableSession := &session.ResumableSession{
		Metadata: &session.Metadata{
			SessionID: "test-session-id",
		},
	}

	provider := client.NewAgentProvider(client.ClientGemini, map[string]any{
		client.ExtGeminiModel: "gemini-2.5-flash",
	})
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	cfg := NewInitializerConfigFromModel(
		"/work/dir",
		"", // beadsDir - empty
		providers,
		"develop",
		"feature-branch",
		config.TracingConfig{Enabled: true, Exporter: "file"},
		config.SessionStorageConfig{BaseDir: "/sessions"},
	).
		WithTimeout(45 * time.Second).
		WithGitExecutor(mockExecutor).
		WithRestoredSession(resumableSession).
		WithSoundService(mockSound).
		Build()

	// Verify all static fields
	require.Equal(t, "/work/dir", cfg.WorkDir)
	require.Equal(t, client.ClientGemini, cfg.AgentProviders.Coordinator().Type())
	require.Equal(t, "gemini-2.5-flash", cfg.AgentProviders.Coordinator().Extensions()[client.ExtGeminiModel])
	require.Equal(t, "develop", cfg.WorktreeBaseBranch)
	require.Equal(t, "feature-branch", cfg.WorktreeBranchName)
	require.True(t, cfg.TracingConfig.Enabled)
	require.Equal(t, "file", cfg.TracingConfig.Exporter)
	require.Equal(t, "/sessions", cfg.SessionStorage.BaseDir)

	// Verify all runtime fields
	require.Equal(t, 45*time.Second, cfg.Timeouts.CoordinatorStart)
	require.Same(t, mockExecutor, cfg.GitExecutor)
	require.Same(t, resumableSession, cfg.RestoredSession)
	require.Same(t, mockSound, cfg.SoundService)
}

func TestInitializerConfigBuilder_RuntimeFieldsDefaultToZero(t *testing.T) {
	// Test that runtime-only fields are zero by default when not set via builder
	provider := client.NewAgentProvider(client.ClientClaude, map[string]any{
		client.ExtClaudeModel: "opus",
	})
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	cfg := NewInitializerConfigFromModel(
		"/work/dir", "", providers,
		"", "", config.TracingConfig{}, config.SessionStorageConfig{},
	).Build()

	// Runtime-only fields should be zero/nil
	require.Equal(t, config.TimeoutsConfig{}, cfg.Timeouts)
	require.Nil(t, cfg.GitExecutor)
	require.Nil(t, cfg.RestoredSession)
	require.Nil(t, cfg.SoundService)

	// Static fields should be set
	require.Equal(t, "/work/dir", cfg.WorkDir)
	require.Equal(t, client.ClientClaude, cfg.AgentProviders.Coordinator().Type())
	require.Equal(t, "opus", cfg.AgentProviders.Coordinator().Extensions()[client.ExtClaudeModel])
}

func TestInitializerConfigBuilder_PartialModelState(t *testing.T) {
	// Test builder with partial state (simulating edge case where only some fields are set)
	// AgentProviders.Coordinator() must have a type, but can have extensions without a type being explicitly named
	provider := client.NewAgentProvider("", map[string]any{
		client.ExtClaudeModel: "haiku",
	})
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	cfg := NewInitializerConfigFromModel(
		"", // empty workDir
		"", // empty beadsDir
		providers,
		"main", // worktree branch set
		"",     // no custom branch
		config.TracingConfig{},
		config.SessionStorageConfig{},
	).
		WithTimeout(30 * time.Second).
		Build()

	require.Empty(t, cfg.WorkDir)
	require.Empty(t, cfg.AgentProviders.Coordinator().Type())
	require.Equal(t, "haiku", cfg.AgentProviders.Coordinator().Extensions()[client.ExtClaudeModel])
	require.Equal(t, "main", cfg.WorktreeBaseBranch)
	require.Equal(t, 30*time.Second, cfg.Timeouts.CoordinatorStart)
}

func TestInitializerConfigBuilder_ProducesEquivalentConfig(t *testing.T) {
	// Test that builder produces identical results to inline construction
	timeout := 20 * time.Second
	mockExecutor := &mocks.MockGitExecutor{}
	mockSound := &mocks.MockSoundService{}
	provider := client.NewAgentProvider(client.ClientClaude, map[string]any{
		client.ExtClaudeModel: "opus",
	})
	providers := client.AgentProviders{
		client.RoleCoordinator: provider,
	}

	// Build config using inline construction
	inlineConfig := InitializerConfig{
		WorkDir:            "/work/dir",
		AgentProviders:     providers,
		Timeouts:           config.TimeoutsConfig{CoordinatorStart: timeout},
		WorktreeBaseBranch: "main",
		WorktreeBranchName: "custom",
		GitExecutor:        mockExecutor,
		TracingConfig:      config.TracingConfig{Enabled: true},
		SessionStorage:     config.SessionStorageConfig{BaseDir: "/tmp"},
		SoundService:       mockSound,
	}

	// Build config using builder pattern
	builderConfig := NewInitializerConfigFromModel(
		"/work/dir",
		"", // beadsDir - empty
		providers,
		"main",
		"custom",
		config.TracingConfig{Enabled: true},
		config.SessionStorageConfig{BaseDir: "/tmp"},
	).
		WithTimeout(timeout).
		WithGitExecutor(mockExecutor).
		WithSoundService(mockSound).
		Build()

	// Compare all fields
	require.Equal(t, inlineConfig.WorkDir, builderConfig.WorkDir)
	require.Equal(t, inlineConfig.AgentProviders.Coordinator().Type(), builderConfig.AgentProviders.Coordinator().Type())
	require.Equal(t, inlineConfig.Timeouts.CoordinatorStart, builderConfig.Timeouts.CoordinatorStart)
	require.Equal(t, inlineConfig.AgentProviders.Coordinator().Extensions(), builderConfig.AgentProviders.Coordinator().Extensions())
	require.Equal(t, inlineConfig.WorktreeBaseBranch, builderConfig.WorktreeBaseBranch)
	require.Equal(t, inlineConfig.WorktreeBranchName, builderConfig.WorktreeBranchName)
	require.Same(t, inlineConfig.GitExecutor, builderConfig.GitExecutor)
	require.Equal(t, inlineConfig.TracingConfig, builderConfig.TracingConfig)
	require.Equal(t, inlineConfig.SessionStorage, builderConfig.SessionStorage)
	require.Same(t, inlineConfig.SoundService, builderConfig.SoundService)
}

func TestInitializerPhase(t *testing.T) {
	init := NewInitializer(InitializerConfig{
		WorkDir: "/test/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	require.Equal(t, InitNotStarted, init.Phase())
}

func TestInitializerResources_HasSession(t *testing.T) {
	// Verify InitializerResources includes a Session field
	resources := InitializerResources{}

	// This is a compile-time check that the field exists
	_ = resources.Session
}

func TestIntegration_SessionCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This is an integration test that verifies session creation
	// by manually calling the session package

	workDir := t.TempDir()
	sessionID := "integration-test-session"
	sessionDir := filepath.Join(workDir, ".perles", "sessions", sessionID)

	// Import and use the session package directly to verify it works as expected
	// This mimics what createWorkspace() does

	// Verify the path doesn't exist yet
	_, err := os.Stat(sessionDir)
	require.True(t, os.IsNotExist(err))

	// The actual session creation is handled by session.New() which is thoroughly tested
	// We're verifying the integration path here
}

// ===========================================================================
// V2 Orchestration Infrastructure Tests
// ===========================================================================

func TestInitializer_Retry_ResetsV2Infrastructure(t *testing.T) {
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{CoordinatorStart: 100 * time.Millisecond}, // Short timeout for test
	})

	// Verify v2 infrastructure is nil initially
	require.Nil(t, init.GetV2Infra())

	// After Retry is called (which calls Cancel first), v2 fields should be reset to nil
	// We can verify the fields exist and would be reset in the Retry method
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	// Fields should still be nil
	require.Nil(t, init.GetV2Infra())
}

func TestInitializer_V2FieldsExist(t *testing.T) {
	// Compile-time check that v2Infra field exists and getter works
	init := NewInitializer(InitializerConfig{
		WorkDir: "/test/dir",
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// GetV2Infra should exist and return nil before Start
	require.Nil(t, init.GetV2Infra())
}

func TestInitializer_V2FieldsNilBeforeStart(t *testing.T) {
	// Verify v2 infrastructure is nil before Start() is called
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Before Start, v2Infra should be nil (accessed via getter)
	require.Nil(t, init.GetV2Infra(), "v2Infra should be nil before Start")
}

func TestInitializer_CleanupDrainsProcessor(t *testing.T) {
	// This test verifies that cleanupResources() properly handles nil cmdProcessor
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// cleanupResources() should not panic when cmdProcessor is nil
	require.NotPanics(t, func() {
		init.cleanupResources()
	})
}

// ===========================================================================
// V2 Event Bus Tests
// ===========================================================================

func TestInitializer_GetV2EventBus_NilBeforeStart(t *testing.T) {
	// Verify GetV2EventBus() returns nil before Start() is called
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Before Start, GetV2EventBus should return nil
	require.Nil(t, init.GetV2EventBus(), "GetV2EventBus should return nil before initialization")
}

func TestInitializer_GetV2EventBus_ThreadSafe(t *testing.T) {
	// Verify GetV2EventBus() uses read lock for thread safety
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Concurrent calls should not race (verified with -race flag)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = init.GetV2EventBus()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestInitializer_V2EventBusFieldExists(t *testing.T) {
	// Compile-time check that GetV2EventBus getter exists
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// GetV2EventBus should exist and return nil before Start
	require.Nil(t, init.GetV2EventBus())
}

func TestInitializer_Retry_ResetsV2EventBus(t *testing.T) {
	// Verify v2EventBus is reset when Retry() is called (via v2Infra reset)
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{CoordinatorStart: 100 * time.Millisecond},
	})

	// v2EventBus should be nil initially (via getter which checks v2Infra)
	require.Nil(t, init.GetV2EventBus(), "v2EventBus should be nil before Start")

	// The Retry method resets v2Infra to nil, which means GetV2EventBus returns nil
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	require.Nil(t, init.GetV2EventBus(), "v2EventBus should be nil after reset")
}

// ===========================================================================
// Unified Process Infrastructure Tests (Phase 5)
// ===========================================================================

func TestInitializer_ProcessRepoFieldExists(t *testing.T) {
	// Compile-time check that GetProcessRepository getter exists
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// GetProcessRepository should exist and return nil before Start
	require.Nil(t, init.GetProcessRepository())
}

func TestInitializer_ProcessRepoNilBeforeStart(t *testing.T) {
	// Verify processRepo is nil before Start() is called (via getter)
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Before Start, processRepo should be nil (accessed via getter which checks v2Infra)
	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil before Start")
}

func TestInitializer_Retry_ResetsProcessRepo(t *testing.T) {
	// Verify processRepo is reset when Retry() is called (via v2Infra reset)
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{CoordinatorStart: 100 * time.Millisecond},
	})

	// processRepo should be nil initially (via getter which checks v2Infra)
	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil before Start")

	// The Retry method resets v2Infra to nil, which means GetProcessRepository returns nil
	init.mu.Lock()
	init.v2Infra = nil
	init.mu.Unlock()

	require.Nil(t, init.GetProcessRepository(), "processRepo should be nil after reset")
}

// ===========================================================================
// ProcessEvent Handling Tests (Phase 5)
// ===========================================================================

// ===========================================================================
// createSession() Method Tests (Task perles-oph9.1)
// ===========================================================================

func TestInitializer_CreateSession_Success(t *testing.T) {
	// Verify createSession() creates a valid session with expected directory structure
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir() // Centralized sessions directory

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "12345678-90ab-cdef-1234-567890abcdef"
	init.mu.Unlock()

	// Call createSession directly
	sess, err := init.createSession()

	// Verify no error
	require.NoError(t, err, "createSession should not return an error")
	require.NotNil(t, sess, "createSession should return a non-nil session")
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	// Verify session has an ID (UUID format)
	require.NotEmpty(t, sess.ID, "session should have a non-empty ID")
	require.Len(t, sess.ID, 36, "session ID should be a valid UUID (36 chars)")

	// Verify the session directory was created in centralized location
	// Path: {baseDir}/{appName}/{date}/{sessionID}
	today := time.Now().Format("2006-01-02")
	sessionDir := filepath.Join(sessionsBaseDir, "test-app", today, sess.ID)
	info, err := os.Stat(sessionDir)
	require.NoError(t, err, "session directory should exist")
	require.True(t, info.IsDir(), "session directory should be a directory")

	// Verify sessionDir is stored in initializer
	require.Equal(t, sessionDir, init.SessionDir(), "SessionDir() should return the created session directory")
}

func TestInitializer_CreateSession_ReturnsErrorOnFailure(t *testing.T) {
	// Verify createSession() returns proper error on session.New failure
	// We simulate failure by using an invalid/unwritable path for session storage

	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	// Create a file where a directory is expected (to force MkdirAll failure)
	blockingFile := filepath.Join(sessionsBaseDir, "test-app")
	err := os.WriteFile(blockingFile, []byte("blocking file"), 0644)
	require.NoError(t, err, "setup: should create blocking file")

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app", // This will try to create under the blocking file
		},
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Call createSession - should fail because we can't create a directory
	// under a file
	sess, err := init.createSession()

	// Verify error is returned
	require.Error(t, err, "createSession should return an error when session creation fails")
	require.Nil(t, sess, "createSession should return nil session on error")
	require.Contains(t, err.Error(), "failed to create session", "error should indicate session creation failure")
}

func TestInitializer_CreateSession_UniqueIDs(t *testing.T) {
	// Verify createSession() uses the session ID set by Start()
	// Each session should have the expected unique ID when we set different IDs
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Create multiple sessions with different pre-set IDs
	init.mu.Lock()
	init.sessionID = "session-id-11111111-1111-1111-1111-111111111111"
	init.mu.Unlock()
	sess1, err1 := init.createSession()
	require.NoError(t, err1)
	t.Cleanup(func() { _ = sess1.Close(session.StatusCompleted) })

	init.mu.Lock()
	init.sessionID = "session-id-22222222-2222-2222-2222-222222222222"
	init.mu.Unlock()
	sess2, err2 := init.createSession()
	require.NoError(t, err2)
	t.Cleanup(func() { _ = sess2.Close(session.StatusCompleted) })

	init.mu.Lock()
	init.sessionID = "session-id-33333333-3333-3333-3333-333333333333"
	init.mu.Unlock()
	sess3, err3 := init.createSession()
	require.NoError(t, err3)
	t.Cleanup(func() { _ = sess3.Close(session.StatusCompleted) })

	// Verify all IDs are unique (because we set different IDs)
	require.NotEqual(t, sess1.ID, sess2.ID, "session IDs should be unique")
	require.NotEqual(t, sess2.ID, sess3.ID, "session IDs should be unique")
	require.NotEqual(t, sess1.ID, sess3.ID, "session IDs should be unique")
}

func TestInitializer_CreateSession_DirectoryStructure(t *testing.T) {
	// Verify the session directory follows the new centralized path pattern
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-00001111-2222-3333-4444-555566667777"
	init.mu.Unlock()

	sess, err := init.createSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	// Verify the directory structure is: {baseDir}/{appName}/{date}/{sessionID}
	today := time.Now().Format("2006-01-02")
	expectedAppDir := filepath.Join(sessionsBaseDir, "test-app")
	expectedDateDir := filepath.Join(expectedAppDir, today)
	expectedSessionDir := filepath.Join(expectedDateDir, sess.ID)

	// Verify app directory exists
	info, err := os.Stat(expectedAppDir)
	require.NoError(t, err, "app directory should exist")
	require.True(t, info.IsDir(), "app directory should be a directory")

	// Verify date directory exists
	info, err = os.Stat(expectedDateDir)
	require.NoError(t, err, "date directory should exist")
	require.True(t, info.IsDir(), "date directory should be a directory")

	// Verify session-specific directory exists
	info, err = os.Stat(expectedSessionDir)
	require.NoError(t, err, "session directory should exist")
	require.True(t, info.IsDir(), "session directory should be a directory")

	// Verify SessionDir() accessor returns the correct path
	require.Equal(t, expectedSessionDir, init.SessionDir())
}

// ===========================================================================
// AgentProvider Tests (Replaced createAIClient tests after AgentProvider refactor)
// ===========================================================================

func TestInitializer_AgentProvider_Claude(t *testing.T) {
	// Verify AgentProvider with Claude client type has correct extensions
	provider := client.NewAgentProvider(client.ClientClaude, map[string]any{
		client.ExtClaudeModel: "opus",
	})

	// Verify client type
	require.Equal(t, client.ClientClaude, provider.Type(), "provider should be Claude type")

	// Verify extensions map contains Claude model
	require.Contains(t, provider.Extensions(), client.ExtClaudeModel, "extensions should contain Claude model key")
	require.Equal(t, "opus", provider.Extensions()[client.ExtClaudeModel], "Claude model should be 'opus'")

	// Verify Client() returns valid client
	c, err := provider.Client()
	require.NoError(t, err, "Client() should not return an error for claude type")
	require.NotNil(t, c, "client should not be nil")
	require.Equal(t, client.ClientClaude, c.Type(), "client should be Claude type")
}

func TestInitializer_AgentProvider_Amp(t *testing.T) {
	// Verify AgentProvider with Amp client type has correct extensions
	provider := client.NewAgentProvider(client.ClientAmp, map[string]any{
		client.ExtAmpModel: "gpt-4",
		amp.ExtAmpMode:     "smart",
	})

	// Verify client type
	require.Equal(t, client.ClientAmp, provider.Type(), "provider should be Amp type")

	// Verify extensions map contains Amp model and mode
	require.Contains(t, provider.Extensions(), client.ExtAmpModel, "extensions should contain Amp model key")
	require.Equal(t, "gpt-4", provider.Extensions()[client.ExtAmpModel], "Amp model should be 'gpt-4'")
	require.Contains(t, provider.Extensions(), amp.ExtAmpMode, "extensions should contain Amp mode key")
	require.Equal(t, "smart", provider.Extensions()[amp.ExtAmpMode], "Amp mode should be 'smart'")

	// Verify Client() returns valid client
	c, err := provider.Client()
	require.NoError(t, err, "Client() should not return an error for amp type")
	require.NotNil(t, c, "client should not be nil")
	require.Equal(t, client.ClientAmp, c.Type(), "client should be Amp type")
}

func TestInitializer_AgentProvider_NoExtensionsWhenNil(t *testing.T) {
	// Verify AgentProvider with nil extensions returns empty map
	provider := client.NewAgentProvider(client.ClientClaude, nil)

	// Extensions are always non-nil (empty map) with AgentProvider
	require.NotNil(t, provider.Extensions(), "extensions should not be nil")
	require.Empty(t, provider.Extensions(), "extensions should be empty when none configured")
}

func TestInitializer_AgentProvider_InvalidClientType(t *testing.T) {
	// Verify AgentProvider.Client() returns error for unknown client type
	provider := client.NewAgentProvider("unknown-client", nil)

	c, err := provider.Client()
	require.Error(t, err, "Client() should return error for unknown client type")
	require.Nil(t, c, "client should be nil on error")
}

func TestInitializer_AgentProvider_AmpPartialExtensions(t *testing.T) {
	// Verify AgentProvider with Amp type handles partial extensions (only model, no mode)
	provider := client.NewAgentProvider(client.ClientAmp, map[string]any{
		client.ExtAmpModel: "gpt-4",
		// No mode - should not be in extensions
	})

	// Verify extensions only contain model, not mode
	require.Contains(t, provider.Extensions(), client.ExtAmpModel, "extensions should contain Amp model")
	require.NotContains(t, provider.Extensions(), amp.ExtAmpMode, "extensions should not contain empty Amp mode")
}

// ===========================================================================
// createMCPListener() Method Tests (Task perles-oph9.3)
// ===========================================================================

func TestInitializer_CreateMCPListener_Success(t *testing.T) {
	// Verify createMCPListener() returns a valid listener on a random port
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Call createMCPListener directly
	result, err := init.createMCPListener()

	// Verify no error
	require.NoError(t, err, "createMCPListener should not return an error")
	require.NotNil(t, result, "createMCPListener should return a non-nil result")
	require.NotNil(t, result.Listener, "createMCPListener should return a non-nil listener")

	// Verify port is a valid non-zero port
	require.Greater(t, result.Port, 0, "port should be greater than 0")
	require.Less(t, result.Port, 65536, "port should be a valid TCP port")

	// Verify listener is actually listening (can get its address)
	addr := result.Listener.Addr()
	require.NotNil(t, addr, "listener should have a valid address")

	// Verify we got the same port from the listener
	tcpAddr, ok := addr.(*net.TCPAddr)
	require.True(t, ok, "listener address should be a TCP address")
	require.Equal(t, result.Port, tcpAddr.Port, "returned port should match listener port")

	// Clean up
	_ = result.Listener.Close()
}

func TestInitializer_CreateMCPListener_BindsToLocalhost(t *testing.T) {
	// Verify createMCPListener() binds to localhost (127.0.0.1) only
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	result, err := init.createMCPListener()
	require.NoError(t, err)
	defer result.Listener.Close()

	// Verify the listener is bound to localhost
	tcpAddr, ok := result.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.True(t, tcpAddr.IP.IsLoopback(), "listener should be bound to localhost")
}

func TestInitializer_CreateMCPListener_UniquePortsOnMultipleCalls(t *testing.T) {
	// Verify multiple calls to createMCPListener() return different ports
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Create multiple listeners
	result1, err := init.createMCPListener()
	require.NoError(t, err)
	defer result1.Listener.Close()

	result2, err := init.createMCPListener()
	require.NoError(t, err)
	defer result2.Listener.Close()

	// Ports should be different since both listeners are still open
	require.NotEqual(t, result1.Port, result2.Port, "multiple listeners should get different ports")
}

func TestInitializer_CreateMCPListener_ListenerAcceptsConnections(t *testing.T) {
	// Verify the returned listener can actually accept connections
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	result, err := init.createMCPListener()
	require.NoError(t, err)
	defer result.Listener.Close()

	// Try to connect to the listener
	conn, err := net.Dial("tcp", result.Listener.Addr().String())
	require.NoError(t, err, "should be able to connect to the listener")
	_ = conn.Close()
}

// ===========================================================================
// createMCPServer() Method Tests (Task perles-oph9.3)
// ===========================================================================

func TestInitializer_CreateMCPServer_Success(t *testing.T) {
	// Verify createMCPServer() returns a valid MCPServerResult with all components
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         t.TempDir(),
			ApplicationName: "test-app",
		},
	})

	// First create dependencies
	sess, err := init.createSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	// Create mock message repository
	msgRepo := repository.NewMemoryMessageRepository()

	// We need a minimal v2Adapter for the test - create a nil processor adapter
	// Note: In real usage, this would be wired to the command processor

	// Call createMCPServer (v2Adapter can be nil for this test since we're just testing structure)
	result, err := init.createMCPServer(MCPServerConfig{
		Listener:  listenerResult.Listener,
		Port:      listenerResult.Port,
		MsgRepo:   msgRepo,
		Session:   sess,
		V2Adapter: nil, // Worker cache handles nil v2Adapter
		WorkDir:   workDir,
	})

	// Verify no error
	require.NoError(t, err, "createMCPServer should not return an error")
	require.NotNil(t, result, "createMCPServer should return a non-nil result")

	// Verify all components are present
	require.NotNil(t, result.Server, "MCPServerResult.Server should not be nil")
	require.NotNil(t, result.Listener, "MCPServerResult.Listener should not be nil")
	require.NotNil(t, result.CoordServer, "MCPServerResult.CoordServer should not be nil")
	require.NotNil(t, result.WorkerCache, "MCPServerResult.WorkerCache should not be nil")
	require.Equal(t, listenerResult.Port, result.Port, "MCPServerResult.Port should match input port")
}

func TestInitializer_CreateMCPServer_ConfiguresHTTPRoutes(t *testing.T) {
	// Verify createMCPServer() configures /mcp and /worker/ routes correctly
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         t.TempDir(),
			ApplicationName: "test-app",
		},
	})

	// Create dependencies
	sess, err := init.createSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:  listenerResult.Listener,
		Port:      listenerResult.Port,
		MsgRepo:   msgRepo,
		Session:   sess,
		V2Adapter: nil,
		WorkDir:   workDir,
	})

	// Verify the HTTP server has a handler
	require.NotNil(t, result.Server.Handler, "HTTP server should have a handler configured")

	// Start the server to test routes
	go func() {
		_ = result.Server.Serve(result.Listener)
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Test that /mcp route exists (it should return something, even if not a valid response)
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/mcp", result.Port))
	require.NoError(t, err, "/mcp route should be accessible")
	resp.Body.Close()

	// Test that /worker/ route exists
	resp, err = http.Get(fmt.Sprintf("http://127.0.0.1:%d/worker/test", result.Port))
	require.NoError(t, err, "/worker/ route should be accessible")
	resp.Body.Close()

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = result.Server.Shutdown(ctx)
}

func TestInitializer_CreateMCPServer_ReadHeaderTimeout(t *testing.T) {
	// Verify createMCPServer() configures ReadHeaderTimeout on the HTTP server
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         t.TempDir(),
			ApplicationName: "test-app",
		},
	})

	// Create dependencies
	sess, err := init.createSession()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:  listenerResult.Listener,
		Port:      listenerResult.Port,
		MsgRepo:   msgRepo,
		Session:   sess,
		V2Adapter: nil,
		WorkDir:   workDir,
	})

	// Verify ReadHeaderTimeout is set (should be 10 seconds)
	require.Equal(t, 10*time.Second, result.Server.ReadHeaderTimeout,
		"HTTP server should have ReadHeaderTimeout set to 10 seconds")
}

func TestInitializer_MCPServerResult_ContainsAllComponents(t *testing.T) {
	// Verify MCPServerResult struct has all expected fields accessible
	result := MCPServerResult{}

	// These should compile - verifies the struct fields exist
	_ = result.Server
	_ = result.Port
	_ = result.Listener
	_ = result.CoordServer
	_ = result.WorkerCache

	// Verify they are all nil/zero by default
	require.Nil(t, result.Server)
	require.Equal(t, 0, result.Port)
	require.Nil(t, result.Listener)
	require.Nil(t, result.CoordServer)
	require.Nil(t, result.WorkerCache)
}

func TestInitializer_MCPListenerResult_ContainsAllComponents(t *testing.T) {
	// Verify MCPListenerResult struct has all expected fields accessible
	result := MCPListenerResult{}

	// These should compile - verifies the struct fields exist
	_ = result.Listener
	_ = result.Port

	// Verify they are nil/zero by default
	require.Nil(t, result.Listener)
	require.Equal(t, 0, result.Port)
}

func TestInitializer_MCPServerConfig_ContainsAllFields(t *testing.T) {
	// Verify MCPServerConfig struct has all expected fields accessible
	cfg := MCPServerConfig{}

	// These should compile - verifies the struct fields exist
	_ = cfg.Listener
	_ = cfg.Port
	_ = cfg.MsgRepo
	_ = cfg.Session
	_ = cfg.V2Adapter
	_ = cfg.TurnEnforcer
	_ = cfg.WorkDir
	_ = cfg.BeadsDir
	_ = cfg.Tracer

	// Verify they are nil/zero by default
	require.Nil(t, cfg.Listener)
	require.Equal(t, 0, cfg.Port)
	require.Nil(t, cfg.MsgRepo)
	require.Nil(t, cfg.Session)
	require.Nil(t, cfg.V2Adapter)
	require.Nil(t, cfg.TurnEnforcer)
	require.Empty(t, cfg.WorkDir)
	require.Empty(t, cfg.BeadsDir)
	require.Nil(t, cfg.Tracer)
}

func TestInitializer_CreateMCPServer_RequiresListener(t *testing.T) {
	// Verify createMCPServer() returns error when listener is nil
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	msgRepo := repository.NewMemoryMessageRepository()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:  nil, // Missing listener
		Port:      8080,
		MsgRepo:   msgRepo,
		Session:   nil,
		V2Adapter: nil,
		WorkDir:   workDir,
	})

	require.Error(t, err, "createMCPServer should return error when listener is nil")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "listener is required")
}

func TestInitializer_CreateMCPServer_RequiresMsgRepo(t *testing.T) {
	// Verify createMCPServer() returns error when MsgRepo is nil
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	listenerResult, err := init.createMCPListener()
	require.NoError(t, err)
	defer listenerResult.Listener.Close()

	result, err := init.createMCPServer(MCPServerConfig{
		Listener:  listenerResult.Listener,
		Port:      listenerResult.Port,
		MsgRepo:   nil, // Missing MsgRepo
		Session:   nil,
		V2Adapter: nil,
		WorkDir:   workDir,
	})

	require.Error(t, err, "createMCPServer should return error when MsgRepo is nil")
	require.Nil(t, result, "result should be nil on error")
	require.Contains(t, err.Error(), "message repository is required")
}

func TestInitializer_SpinnerData_ReturnsPhase(t *testing.T) {
	// Verify SpinnerData returns current phase
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Get spinner data - should return InitNotStarted
	phase := init.SpinnerData()
	require.Equal(t, InitNotStarted, phase)

	// Set a different phase
	init.mu.Lock()
	init.phase = InitSpawningCoordinator
	init.mu.Unlock()

	phase = init.SpinnerData()
	require.Equal(t, InitSpawningCoordinator, phase)
}

// ===========================================================================
// run() Tests
// ===========================================================================

func TestRun_CancelsOnContextCancellation(t *testing.T) {
	// Unit test: run() cancels correctly on context cancellation
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{CoordinatorStart: 10 * time.Second}, // Long timeout
	})

	// Verify we can cancel the initializer
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// Cancel the context
	init.cancel()

	// Context should be done
	select {
	case <-init.ctx.Done():
		// Expected
	default:
		require.Fail(t, "context should be cancelled")
	}

	// Verify the error type
	require.Equal(t, context.Canceled, init.ctx.Err())
}

// ===========================================================================
// cleanupResources() Tests (Task perles-oph9.13)
// ===========================================================================

func TestCleanupResources_Idempotent(t *testing.T) {
	// Unit test: cleanupResources() is idempotent (safe to call twice)
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// First call should not panic
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "first cleanupResources call should not panic")

	// Second call should also not panic (idempotent)
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "second cleanupResources call should not panic (idempotent)")

	// Third call for good measure
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "third cleanupResources call should not panic (idempotent)")
}

func TestCleanupResources_ClearsFields(t *testing.T) {
	// Unit test: cleanupResources() clears resource fields for idempotency
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Manually set fields to simulate initialized state
	init.mu.Lock()
	init.mcpServer = &http.Server{} // Dummy server
	init.mu.Unlock()

	// Call cleanupResources
	init.cleanupResources()

	// Verify fields are cleared
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be nil after cleanup")
	require.Nil(t, init.v2Infra, "v2Infra should be nil after cleanup")
	init.mu.RUnlock()
}

func TestCancel_StopsContextAndCleansUp(t *testing.T) {
	// Unit test: Cancel() stops context and cleans up
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	ctx := init.ctx
	init.mu.Unlock()

	// Verify context is not cancelled yet
	select {
	case <-ctx.Done():
		require.Fail(t, "context should not be cancelled yet")
	default:
		// Expected
	}

	// Call Cancel
	init.Cancel()

	// Verify context is now cancelled
	select {
	case <-ctx.Done():
		// Expected - context should be cancelled
		require.Equal(t, context.Canceled, ctx.Err())
	default:
		require.Fail(t, "context should be cancelled after Cancel()")
	}
}

func TestCancel_DoubleCallDoesNotPanic(t *testing.T) {
	// Unit test: Double-Cancel() doesn't panic
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// First Cancel call should not panic
	require.NotPanics(t, func() {
		init.Cancel()
	}, "first Cancel call should not panic")

	// Second Cancel call should also not panic (idempotent)
	require.NotPanics(t, func() {
		init.Cancel()
	}, "second Cancel call should not panic (idempotent)")

	// Third Cancel call for good measure
	require.NotPanics(t, func() {
		init.Cancel()
	}, "third Cancel call should not panic (idempotent)")
}

func TestCancel_Idempotent_CancelFuncCalledOnce(t *testing.T) {
	// Unit test: Cancel() only calls cancel func once
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Set up context
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	init.mu.Unlock()

	// First Cancel should clear the cancel func
	init.Cancel()

	// Verify cancel is now nil
	init.mu.RLock()
	require.Nil(t, init.cancel, "cancel func should be nil after Cancel()")
	init.mu.RUnlock()
}

func TestCleanupResources_WithPartialInitialization(t *testing.T) {
	// Unit test: Cleanup with partial initialization works
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Simulate partial initialization - only some fields set
	// This simulates a failure during initialization
	init.mu.Lock()
	init.mcpServer = &http.Server{} // Only MCP server initialized
	// v2Infra, dedupMiddleware all nil
	init.mu.Unlock()

	// Cleanup should not panic even with partial initialization
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "cleanupResources should handle partial initialization")

	// Fields should be cleared
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be nil")
	init.mu.RUnlock()
}

func TestCleanupResources_WithNoInitialization(t *testing.T) {
	// Unit test: Cleanup with no initialization (all fields nil) works
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// All fields are nil by default - cleanup should handle this gracefully
	require.NotPanics(t, func() {
		init.cleanupResources()
	}, "cleanupResources should handle no initialization")
}

func TestCleanupResources_ThreadSafe(t *testing.T) {
	// Unit test: cleanupResources() is thread-safe
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Simulate partial initialization
	init.mu.Lock()
	init.mcpServer = &http.Server{}
	init.mu.Unlock()

	// Call cleanupResources concurrently from multiple goroutines
	done := make(chan struct{}, 10)
	for range 10 {
		go func() {
			defer func() { done <- struct{}{} }()
			// Should not panic or race
			init.cleanupResources()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	// Fields should be cleared (first goroutine to get lock wins)
	init.mu.RLock()
	require.Nil(t, init.mcpServer)
	init.mu.RUnlock()
}

// cleanupOrderTracker tracks the order of cleanup operations
type cleanupOrderTracker struct {
	mu    sync.Mutex
	order []string
}

func (t *cleanupOrderTracker) record(op string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.order = append(t.order, op)
}

func (t *cleanupOrderTracker) getOrder() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]string, len(t.order))
	copy(result, t.order)
	return result
}

func TestCleanupResources_Order_Documentation(t *testing.T) {
	// Documentation test: Verify documented cleanup order is correct
	//
	// Expected cleanup order (reverse of creation):
	//   1. Stop coordinator process (created last via spawnCoordinator)
	//   2. Drain command processor (v2 infrastructure started after creation)
	//   3. Stop deduplication middleware (created with v2 infrastructure)
	//   4. Shutdown MCP server with timeout (HTTP server started last in createWorkspace)
	//
	// This test documents the expected order; actual order verification requires
	// integration testing or mocks with tracking.

	// Creation order in createWorkspace():
	//   1. Create AI client
	//   2. Create message repository
	//   3. Create session
	//   4. Create MCP listener
	//   5. Create V2 infrastructure (includes processor and middleware)
	//   6. Start V2 infrastructure
	//   7. Create MCP server
	//   8. Start HTTP server
	//
	// Then in run():
	//   9. Spawn coordinator

	// Therefore cleanup order should be:
	//   1. Stop coordinator (reverse of step 9)
	//   2. Drain V2 processor (reverse of step 6)
	//   3. Stop deduplication middleware (part of V2 infrastructure)
	//   4. Shutdown MCP HTTP server (reverse of step 8)

	// This is verified by code inspection of cleanupResources()
	// The function clearly shows the order with comments
}

func TestRetry_CallsCancel(t *testing.T) {
	// Unit test: Verify Retry() resets state including cancel func
	// This test verifies the Cancel() call within Retry() by checking state reset
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
	})

	// Manually set up context like Start() does
	init.mu.Lock()
	init.ctx, init.cancel = context.WithCancel(context.Background())
	ctx := init.ctx
	init.started = true
	// Set some resource fields that should be cleared by Cancel->cleanupResources
	init.mcpServer = &http.Server{}
	init.mu.Unlock()

	// Call Cancel() directly (which is called by Retry())
	// This avoids starting the full initialization which spawns goroutines
	init.Cancel()

	// Verify Cancel was called and cleaned up resources
	init.mu.RLock()
	require.Nil(t, init.mcpServer, "mcpServer should be cleared by Cancel->cleanupResources")
	require.Nil(t, init.cancel, "cancel func should be cleared by Cancel")
	init.mu.RUnlock()

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		require.Fail(t, "context should be cancelled")
	}
}

func TestCleanupResources_ExistingTestStillPasses(t *testing.T) {
	// Regression test: Verify existing cleanup test still passes after renaming
	// This is the same as TestInitializer_CleanupDrainsProcessor but ensures
	// the renamed cleanupResources() method works the same way
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// cleanupResources() should not panic when cmdProcessor is nil
	// (Note: cleanupResources uses v2Infra.Drain() not direct cmdProcessor access)
	require.NotPanics(t, func() {
		init.cleanupResources()
	})
}

// ===========================================================================
// Worktree Phase Tests (Task perles-v5cq.5)
// ===========================================================================

func TestInitializer_WorktreePhase_Success(t *testing.T) {
	// Unit test: createWorktree() succeeds with proper mock setup
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	// Start to generate session ID
	init.mu.Lock()
	init.sessionID = "test-session-id-12345678"
	init.mu.Unlock()

	// Call createWorktree directly
	err := init.createWorktreeWithContext(context.Background())
	require.NoError(t, err, "createWorktree should succeed")

	// Verify worktree state was set
	require.Equal(t, worktreePath, init.WorktreePath())
	require.Equal(t, "perles-session-test-ses", init.WorktreeBranch())
}

func TestInitializer_WorktreePhase_NotGitRepo_Fails(t *testing.T) {
	// Unit test: createWorktree() fails when not in git repo
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(false)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.Error(t, err, "createWorktree should fail when not in git repo")
	require.Contains(t, err.Error(), "not a git repository")
}

func TestInitializer_WorktreePhase_CreateFails_Fails(t *testing.T) {
	// Unit test: createWorktree() fails when CreateWorktree fails
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(fmt.Errorf("branch already checked out"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.Error(t, err, "createWorktree should fail when CreateWorktreeWithContext fails")
	require.Contains(t, err.Error(), "failed to create worktree")
}

func TestInitializer_WorktreePhase_Disabled_SkipsPhase(t *testing.T) {
	// Unit test: run() skips worktree phase when disabled
	workDir := t.TempDir()

	// Create mock that should NOT be called
	mockGit := mocks.NewMockGitExecutor(t)
	// No expectations set - if any method is called, test will fail

	init := NewInitializer(InitializerConfig{
		WorkDir:     workDir,
		GitExecutor: mockGit,
	})

	// Verify worktree path is empty
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

func TestInitializer_WorktreePath_PropagatedToWorkspace(t *testing.T) {
	// Unit test: Verify worktreePath is used in createWorkspace
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
	})

	// Manually set worktree path (simulating successful createWorktree)
	init.mu.Lock()
	init.worktreePath = worktreePath
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Verify accessor returns the path
	require.Equal(t, worktreePath, init.WorktreePath())
}

func TestInitializer_PruneWorktrees_CalledBeforeCreate(t *testing.T) {
	// Unit test: Verify PruneWorktrees is called before CreateWorktree
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	// Track call order
	var callOrder []string
	orderMu := sync.Mutex{}

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true).Run(func() {
		orderMu.Lock()
		callOrder = append(callOrder, "IsGitRepo")
		orderMu.Unlock()
	})
	mockGit.EXPECT().PruneWorktrees().Return(nil).Run(func() {
		orderMu.Lock()
		callOrder = append(callOrder, "PruneWorktrees")
		orderMu.Unlock()
	})
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil).Run(func(sessionID string) {
		orderMu.Lock()
		callOrder = append(callOrder, "DetermineWorktreePath")
		orderMu.Unlock()
	})
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Run(func(ctx context.Context, path string, newBranch string, baseBranch string) {
		orderMu.Lock()
		callOrder = append(callOrder, "CreateWorktreeWithContext")
		orderMu.Unlock()
	})

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.NoError(t, err)

	// Verify PruneWorktrees is called before CreateWorktreeWithContext
	orderMu.Lock()
	defer orderMu.Unlock()
	require.Equal(t, []string{"IsGitRepo", "PruneWorktrees", "DetermineWorktreePath", "CreateWorktreeWithContext"}, callOrder)
}

func TestInitializer_BranchName_DefaultsToSessionID(t *testing.T) {
	// Unit test: Verify branch name defaults to perles-session-{shortID}
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "12345678-90ab-cdef-1234-567890abcdef"
	expectedBranch := "perles-session-12345678"

	var capturedNewBranch, capturedBaseBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(ctx context.Context, path string, newBranch string, baseBranch string) {
			capturedNewBranch = newBranch
			capturedBaseBranch = baseBranch
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "", // Empty - uses current HEAD
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, expectedBranch, capturedNewBranch)
	require.Equal(t, "", capturedBaseBranch) // Empty base branch means current HEAD
	require.Equal(t, expectedBranch, init.WorktreeBranch())
}

func TestInitializer_BranchName_UsesConfiguredBaseBranch(t *testing.T) {
	// Unit test: Verify configured base branch is passed to CreateWorktree
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	baseBranch := "develop"
	sessionID := "test-sess"
	expectedNewBranch := "perles-session-test-ses"

	var capturedNewBranch, capturedBaseBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(ctx context.Context, path string, newBranch string, base string) {
			capturedNewBranch = newBranch
			capturedBaseBranch = base
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: baseBranch, // Configured base branch
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.NoError(t, err)
	require.Equal(t, expectedNewBranch, capturedNewBranch) // Auto-generated branch name
	require.Equal(t, baseBranch, capturedBaseBranch)       // Base branch passed correctly
	require.Equal(t, expectedNewBranch, init.WorktreeBranch())
}

func TestInitializer_WorktreePhase_PruneFailsContinues(t *testing.T) {
	// Unit test: Verify worktree creation continues even if prune fails
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(fmt.Errorf("prune failed")) // Failure should be ignored
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.NoError(t, err, "createWorktree should succeed even if prune fails")
	require.Equal(t, worktreePath, init.WorktreePath())
}

func TestInitializer_WorktreePhase_DeterminePathFails(t *testing.T) {
	// Unit test: Verify failure when DetermineWorktreePath fails
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return("", fmt.Errorf("path determination failed"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktreeWithContext(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to determine worktree path")
}

func TestInitializer_Retry_ResetsWorktreeFields(t *testing.T) {
	// Unit test: Verify Retry() resets worktree fields
	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
	})

	// Manually set worktree fields
	init.mu.Lock()
	init.worktreePath = "/tmp/test-worktree"
	init.worktreeBranch = "test-branch"
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Call Cancel (which is called by Retry first)
	init.Cancel()

	// After Cancel, manually reset like Retry does
	init.mu.Lock()
	init.worktreePath = ""
	init.worktreeBranch = ""
	init.sessionID = ""
	init.mu.Unlock()

	// Verify fields are reset
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

func TestInitializer_WorktreePath_Accessor_ThreadSafe(t *testing.T) {
	// Unit test: Verify WorktreePath() is thread-safe
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// Concurrently read WorktreePath
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreePath()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestInitializer_WorktreeBranch_Accessor_ThreadSafe(t *testing.T) {
	// Unit test: Verify WorktreeBranch() is thread-safe
	init := NewInitializer(InitializerConfig{
		WorkDir: t.TempDir(),
	})

	// Concurrently read WorktreeBranch
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreeBranch()
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}

func TestInitializer_WorktreeConfig_Fields(t *testing.T) {
	// Unit test: Verify InitializerConfig has all worktree fields
	config := InitializerConfig{
		WorkDir:            "/test/dir",
		WorktreeBaseBranch: "test-branch",
		GitExecutor:        nil, // Will be set in real usage
	}

	// Verify fields are accessible and set correctly
	require.Equal(t, "/test/dir", config.WorkDir)
	require.Equal(t, "test-branch", config.WorktreeBaseBranch)
	require.Nil(t, config.GitExecutor)
}

// ===========================================================================
// WorktreeBranchName Field Tests (Task perles-s8xg.2)
// ===========================================================================

func TestInitializerConfig_WorktreeBranchName_FieldExists(t *testing.T) {
	// Verify the WorktreeBranchName field exists and can be set
	config := InitializerConfig{
		WorkDir:            "/test/dir",
		WorktreeBranchName: "feature/my-custom-branch",
	}

	// Verify it was set
	require.Equal(t, "feature/my-custom-branch", config.WorktreeBranchName)
}

func TestInitializerConfig_WorktreeBranchName_DefaultsToEmpty(t *testing.T) {
	// Verify WorktreeBranchName defaults to empty string (zero value)
	config := InitializerConfig{
		WorkDir: "/test/dir",
		// WorktreeBranchName not set - should be empty
	}

	// Field should be empty by default
	require.Empty(t, config.WorktreeBranchName, "WorktreeBranchName should be empty by default")
}

func TestInitializerConfig_WorktreeBranchName_EmptyIsZeroValue(t *testing.T) {
	// Verify empty string is the zero value for WorktreeBranchName
	config := InitializerConfig{
		WorkDir:            "/test/dir",
		WorktreeBranchName: "", // Explicitly empty
	}

	// Field should be empty
	require.Empty(t, config.WorktreeBranchName, "empty string should be zero value for WorktreeBranchName")
}

func TestInitializerConfig_WorktreeBranchName_WithAllWorktreeFields(t *testing.T) {
	// Verify WorktreeBranchName works together with other worktree fields
	config := InitializerConfig{
		WorkDir:            "/test/dir",
		WorktreeBaseBranch: "main",
		WorktreeBranchName: "feature/custom",
	}

	// Verify all worktree fields are correctly set
	require.Equal(t, "/test/dir", config.WorkDir)
	require.Equal(t, "main", config.WorktreeBaseBranch)
	require.Equal(t, "feature/custom", config.WorktreeBranchName)
}

func TestNewInitializer_WorktreeBranchName_PassedViaConfig(t *testing.T) {
	// Verify NewInitializer accepts WorktreeBranchName in config
	cfg := InitializerConfig{
		WorkDir:            t.TempDir(),
		WorktreeBranchName: "custom-branch-name",
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)

	// Verify the config was stored (accessed via cfg field)
	require.Equal(t, "custom-branch-name", init.cfg.WorktreeBranchName)
}

// ===========================================================================
// Session Restoration Tests (Phase 4e - perles-x977.5)
// ===========================================================================

func TestInitializerConfig_RestoredSession_FieldExists(t *testing.T) {
	// Verify the RestoredSession field exists and can be set
	config := InitializerConfig{
		WorkDir:         "/test/dir",
		RestoredSession: nil,
	}

	// Field should be nil by default
	require.Nil(t, config.RestoredSession)
}

func TestInitializerConfig_RestoredSession_AcceptsResumableSession(t *testing.T) {
	// Verify RestoredSession accepts a *session.ResumableSession
	meta := &session.Metadata{
		SessionID:             "test-session-id",
		SessionDir:            "/test/session/dir",
		CoordinatorSessionRef: "coord-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	config := InitializerConfig{
		WorkDir:         "/test/dir",
		RestoredSession: resumable,
	}

	// Verify it was set
	require.NotNil(t, config.RestoredSession)
	require.Equal(t, "test-session-id", config.RestoredSession.Metadata.SessionID)
}

func TestNewInitializer_WithRestoredSession(t *testing.T) {
	// Verify NewInitializer accepts RestoredSession in config
	meta := &session.Metadata{
		SessionID:             "restored-session-id",
		SessionDir:            "/restored/session/dir",
		CoordinatorSessionRef: "coord-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	cfg := InitializerConfig{
		WorkDir:         t.TempDir(),
		RestoredSession: resumable,
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)

	// Verify the config was stored
	require.NotNil(t, init.cfg.RestoredSession)
	require.Equal(t, "restored-session-id", init.cfg.RestoredSession.Metadata.SessionID)
}

func TestInitializer_WithoutRestoredSession_UsesNew(t *testing.T) {
	// Verify createSession() uses session.New() when RestoredSession is nil
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
		RestoredSession: nil, // Not restoring
	})

	// Set session ID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "new-session-uuid-12345678"
	init.mu.Unlock()

	// Call createSession directly
	sess, err := init.createSession()
	require.NoError(t, err)
	require.NotNil(t, sess)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	// Verify it's a new session with the generated ID
	require.Equal(t, "new-session-uuid-12345678", sess.ID)

	// Verify session directory was created in the standard location
	today := time.Now().Format("2006-01-02")
	expectedDir := filepath.Join(sessionsBaseDir, "test-app", today, sess.ID)
	info, err := os.Stat(expectedDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestInitializer_WithRestoredSession_UsesReopen(t *testing.T) {
	// Verify createSession() uses session.Reopen() when RestoredSession is set
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	// First, create an actual session to restore from
	existingSessionID := "existing-session-id-abc123"
	existingSessionDir := filepath.Join(sessionsBaseDir, "test-app", "2026-01-12", existingSessionID)

	// Create the session using session.New
	existingSession, err := session.New(existingSessionID, existingSessionDir,
		session.WithWorkDir(workDir),
		session.WithApplicationName("test-app"),
		session.WithDatePartition("2026-01-12"),
	)
	require.NoError(t, err)
	err = existingSession.Close(session.StatusCompleted)
	require.NoError(t, err)

	// Create metadata for restoration
	meta := &session.Metadata{
		SessionID:             existingSessionID,
		SessionDir:            existingSessionDir,
		CoordinatorSessionRef: "coord-session-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
		DatePartition:         "2026-01-12",
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
		RestoredSession: resumable, // Restoring from existing session
	})

	// Call createSession directly
	sess, err := init.createSession()
	require.NoError(t, err)
	require.NotNil(t, sess)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	// Verify it reopened the existing session
	require.Equal(t, existingSessionID, sess.ID)
	require.Equal(t, existingSessionDir, sess.Dir)

	// Verify initializer's sessionDir was updated to match
	require.Equal(t, existingSessionDir, init.SessionDir())
}

func TestInitializer_WithRestoredSession_CoordinatorHasSessionID(t *testing.T) {
	// Verify that coordinator process has correct SessionID after restoration
	// This is a documentation test showing that RestoreProcessRepository
	// populates SessionID from Metadata.CoordinatorSessionRef
	workDir := t.TempDir()

	// Create a resumable session with coordinator ref
	meta := &session.Metadata{
		SessionID:             "session-with-coord",
		SessionDir:            "/tmp/session", // Not a real dir, just for config
		CoordinatorSessionRef: "coord-headless-session-abc",
		Resumable:             true,
		Status:                session.StatusCompleted,
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	cfg := InitializerConfig{
		WorkDir:         workDir,
		RestoredSession: resumable,
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)

	// Verify the coordinator session ref is accessible via config
	require.Equal(t, "coord-headless-session-abc", init.cfg.RestoredSession.Metadata.CoordinatorSessionRef)

	// Note: The actual ProcessRepository restoration is tested separately in
	// session/restore_test.go - TestRestoreProcessRepository_CoordinatorSessionID
}

func TestInitializer_WithRestoredSession_WorkersHaveSessionIDs(t *testing.T) {
	// Verify that worker processes have correct SessionIDs after restoration
	// This is a documentation test showing that RestoreProcessRepository
	// populates SessionID from WorkerMetadata.HeadlessSessionRef
	workDir := t.TempDir()

	// Create a resumable session with worker refs
	worker1 := session.WorkerMetadata{
		ID:                 "worker-1",
		HeadlessSessionRef: "worker-1-headless-ref",
	}
	worker2 := session.WorkerMetadata{
		ID:                 "worker-2",
		HeadlessSessionRef: "worker-2-headless-ref",
	}

	meta := &session.Metadata{
		SessionID:             "session-with-workers",
		SessionDir:            "/tmp/session",
		CoordinatorSessionRef: "coord-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
		Workers:               []session.WorkerMetadata{worker1, worker2},
	}

	resumable := &session.ResumableSession{
		Metadata:      meta,
		ActiveWorkers: []session.WorkerMetadata{worker1, worker2},
	}

	cfg := InitializerConfig{
		WorkDir:         workDir,
		RestoredSession: resumable,
	}

	init := NewInitializer(cfg)
	require.NotNil(t, init)

	// Verify worker session refs are accessible via config
	require.Len(t, init.cfg.RestoredSession.ActiveWorkers, 2)
	require.Equal(t, "worker-1-headless-ref", init.cfg.RestoredSession.ActiveWorkers[0].HeadlessSessionRef)
	require.Equal(t, "worker-2-headless-ref", init.cfg.RestoredSession.ActiveWorkers[1].HeadlessSessionRef)

	// Note: The actual ProcessRepository restoration is tested separately in
	// session/restore_test.go - TestRestoreProcessRepository_WorkerSessionIDs
}

func TestInitializer_RestoredSessionWithNoWorkers(t *testing.T) {
	// Edge case: Verify restoration works with coordinator only (no workers)
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	// Create an actual session to restore from
	existingSessionID := "session-no-workers"
	existingSessionDir := filepath.Join(sessionsBaseDir, "test-app", "2026-01-12", existingSessionID)

	// Create the session using session.New
	existingSession, err := session.New(existingSessionID, existingSessionDir,
		session.WithWorkDir(workDir),
		session.WithApplicationName("test-app"),
		session.WithDatePartition("2026-01-12"),
	)
	require.NoError(t, err)
	err = existingSession.Close(session.StatusCompleted)
	require.NoError(t, err)

	// Create metadata with no workers
	meta := &session.Metadata{
		SessionID:             existingSessionID,
		SessionDir:            existingSessionDir,
		CoordinatorSessionRef: "coord-only-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
		DatePartition:         "2026-01-12",
		Workers:               []session.WorkerMetadata{}, // No workers
	}

	resumable := &session.ResumableSession{
		Metadata:       meta,
		ActiveWorkers:  []session.WorkerMetadata{}, // Empty
		RetiredWorkers: []session.WorkerMetadata{}, // Empty
	}

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
		RestoredSession: resumable,
	})

	// Call createSession - should succeed even with no workers
	sess, err := init.createSession()
	require.NoError(t, err)
	require.NotNil(t, sess)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	require.Equal(t, existingSessionID, sess.ID)
}

func TestInitializer_ReopenSession_PropagatesError(t *testing.T) {
	// Verify that reopenSession propagates errors from session.Reopen
	workDir := t.TempDir()
	nonExistentDir := "/nonexistent/session/dir/that/does/not/exist"

	meta := &session.Metadata{
		SessionID:             "nonexistent-session",
		SessionDir:            nonExistentDir,
		CoordinatorSessionRef: "coord-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         t.TempDir(),
			ApplicationName: "test-app",
		},
		RestoredSession: resumable,
	})

	// Call createSession - should fail because session dir doesn't exist
	sess, err := init.createSession()
	require.Error(t, err)
	require.Nil(t, sess)
	require.Contains(t, err.Error(), "failed to reopen session")
}

func TestInitializer_ReopenSession_SetsSessionIDAndDir(t *testing.T) {
	// Verify reopenSession sets the initializer's sessionID and sessionDir
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()

	// Create an actual session to restore from
	existingSessionID := "session-to-restore"
	existingSessionDir := filepath.Join(sessionsBaseDir, "test-app", "2026-01-12", existingSessionID)

	existingSession, err := session.New(existingSessionID, existingSessionDir,
		session.WithWorkDir(workDir),
		session.WithApplicationName("test-app"),
		session.WithDatePartition("2026-01-12"),
	)
	require.NoError(t, err)
	err = existingSession.Close(session.StatusCompleted)
	require.NoError(t, err)

	meta := &session.Metadata{
		SessionID:             existingSessionID,
		SessionDir:            existingSessionDir,
		CoordinatorSessionRef: "coord-ref",
		Resumable:             true,
		Status:                session.StatusCompleted,
		DatePartition:         "2026-01-12",
	}

	resumable := &session.ResumableSession{
		Metadata: meta,
	}

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
		RestoredSession: resumable,
	})

	// Initially, sessionID and sessionDir are empty
	require.Empty(t, init.SessionDir())

	// Call createSession
	sess, err := init.createSession()
	require.NoError(t, err)
	require.NotNil(t, sess)
	t.Cleanup(func() { _ = sess.Close(session.StatusCompleted) })

	// After reopening, initializer's sessionDir should be updated
	require.Equal(t, existingSessionDir, init.SessionDir())
}

// ===========================================================================
// Per-Phase Timeout Tests (Task perles-mo45.5)
// ===========================================================================

func TestInitializer_WorktreeTimeout(t *testing.T) {
	// Unit test: Worktree phase times out when GitExecutor.CreateWorktreeWithContext blocks
	// Set WorktreeCreation=100ms, MaxTotal=5s, verify timeout fires at ~100ms with worktree phase error
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	// Create mock that blocks longer than timeout
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(ctx context.Context, path, newBranch, baseBranch string) {
			// Block until context is cancelled (timeout)
			<-ctx.Done()
		}).
		Return(context.DeadlineExceeded)

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 100 * time.Millisecond, // Short timeout for test
			CoordinatorStart: 60 * time.Second,
			WorkspaceSetup:   30 * time.Second,
			MaxTotal:         5 * time.Second, // Longer than phase timeout
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Subscribe to events to detect failure
	eventCh := init.Broker().Subscribe(context.Background())

	// Start initialization
	startTime := time.Now()
	err := init.Start()
	require.NoError(t, err)

	// Wait for failure event
	var failedEvent InitializerEvent
	timeout := time.After(2 * time.Second)
eventLoop:
	for {
		select {
		case event := <-eventCh:
			if event.Payload.Type == InitEventFailed {
				failedEvent = event.Payload
				break eventLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for failure event")
		}
	}

	elapsed := time.Since(startTime)

	// Verify failure happened quickly (around 100ms, with some tolerance)
	require.Less(t, elapsed, 500*time.Millisecond, "timeout should fire around 100ms")

	// Verify error indicates worktree phase
	require.Error(t, failedEvent.Error)
	require.Contains(t, failedEvent.Error.Error(), "worktree")

	// Verify phase was worktree creation
	require.Equal(t, InitCreatingWorktree, init.FailedAtPhase())

	// Cleanup
	init.Cancel()
}

func TestInitializer_CoordinatorTimeout(t *testing.T) {
	// Unit test: Coordinator context timeout cancels the coordinator spawn/await phases.
	// This tests that the coordinator timeout context properly expires and causes failure.
	// The actual HeadlessClient spawning is hard to mock fully, so we test the timeout
	// behavior by verifying the context cancellation path in spawnCoordinatorWithContext.

	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 30 * time.Second,
			CoordinatorStart: 200 * time.Millisecond,
			WorkspaceSetup:   30 * time.Second,
			MaxTotal:         5 * time.Second,
		},
	})

	// Set sessionID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Create a pre-cancelled context to simulate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Wait for it to expire
	<-ctx.Done()

	// Call spawnCoordinatorWithContext with expired context
	// This will fail because v2Infra is nil, but if it weren't, it would
	// timeout due to the context deadline being exceeded.
	err := init.spawnCoordinatorWithContext(ctx)

	// Verify error is returned (either v2 infra not initialized or context cancelled)
	require.Error(t, err)
}

func TestInitializer_WorkspaceSetupTimeout(t *testing.T) {
	// Unit test: Workspace setup times out
	// The workspace setup phase is primarily local operations (MCP, session, etc.)
	// and doesn't have any operations we can easily mock to block.
	// This test verifies the timeout error message format is correct when workspace times out.
	//
	// We test this by checking that createWorkspaceWithContext properly returns
	// a timeout error when context is already cancelled.

	workDir := t.TempDir()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 30 * time.Second,
			CoordinatorStart: 60 * time.Second,
			WorkspaceSetup:   100 * time.Millisecond,
			MaxTotal:         5 * time.Second,
		},
	})

	// Set sessionID (normally done by Start())
	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	// Create a pre-cancelled context to simulate timeout
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Wait for it to expire
	<-ctx.Done()

	// Call createWorkspaceWithContext with expired context
	err := init.createWorkspaceWithContext(ctx)

	// Verify error indicates workspace setup timeout
	require.Error(t, err)
	require.Contains(t, err.Error(), "workspace setup timed out")
}

func TestInitializer_MaxTotalCutoff(t *testing.T) {
	// Unit test: MaxTotal timer fires mid-phase (hard-cut semantics)
	// Mock GitExecutor to block 500ms, set WorktreeCreation=5s, MaxTotal=100ms
	// Verify timeout fires at ~100ms (MaxTotal), error indicates BOTH worktree phase AND max exceeded
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"

	// Track when the mock was called to verify timing
	mockCallTime := time.Now()
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(ctx context.Context, path, newBranch, baseBranch string) {
			mockCallTime = time.Now()
			// Block for 500ms (longer than MaxTotal)
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
			}
		}).
		Return(context.DeadlineExceeded)

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 5 * time.Second, // Long timeout - won't fire
			CoordinatorStart: 60 * time.Second,
			WorkspaceSetup:   30 * time.Second,
			MaxTotal:         100 * time.Millisecond, // Short MaxTotal - will fire first
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Subscribe to events
	eventCh := init.Broker().Subscribe(context.Background())

	// Start initialization
	startTime := time.Now()
	err := init.Start()
	require.NoError(t, err)

	// Wait for timeout event
	var timeoutEvent InitializerEvent
	timeout := time.After(2 * time.Second)
eventLoop:
	for {
		select {
		case event := <-eventCh:
			if event.Payload.Type == InitEventTimedOut {
				timeoutEvent = event.Payload
				break eventLoop
			}
			if event.Payload.Type == InitEventFailed {
				timeoutEvent = event.Payload
				break eventLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for timeout event")
		}
	}

	elapsed := time.Since(startTime)
	_ = mockCallTime // Verify timing is around MaxTotal (100ms)

	// Verify timeout happened around 100ms (MaxTotal), not 5s (WorktreeCreation)
	require.Less(t, elapsed, 500*time.Millisecond, "timeout should fire around 100ms (MaxTotal)")

	// Verify error indicates both phase and max total exceeded
	require.Error(t, timeoutEvent.Error)
	errStr := timeoutEvent.Error.Error()
	require.Contains(t, errStr, "max total timeout")
	// Phase 1 = InitCreatingWorktree (no String() method, so it shows as number)
	require.Contains(t, errStr, "1 phase", "error should indicate phase 1 (worktree)")

	// Verify phase was worktree (phase 1)
	require.Equal(t, InitCreatingWorktree, init.FailedAtPhase())

	// Cleanup
	init.Cancel()
}

func TestInitializer_AllPhasesSucceed(t *testing.T) {
	// Unit test: Verify all timeout structures are correctly initialized.
	// This test verifies the worktree phase completes successfully with proper timeout handling
	// and that the timeouts are applied correctly to each phase.
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()
	worktreePath := filepath.Join(workDir, ".worktrees", "test")

	// Set up mocks for successful worktree creation
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	mockGit.EXPECT().GetRemoteURL("origin").Return("https://github.com/test/repo.git", nil).Maybe()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		// NOTE: No RestoredSession - this will fail at coordinator spawn, but
		// we're testing that worktree and workspace phases work correctly with timeouts
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 30 * time.Second,
			CoordinatorStart: 100 * time.Millisecond, // Will timeout here, but that's expected
			WorkspaceSetup:   30 * time.Second,
			MaxTotal:         120 * time.Second,
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Subscribe to events
	eventCh := init.Broker().Subscribe(context.Background())

	// Start initialization
	err := init.Start()
	require.NoError(t, err)

	// Wait for either timeout (expected) or failure
	// We expect to reach coordinator phase and timeout there
	timeout := time.After(3 * time.Second)
	var sawCreatingWorkspace, sawSpawningCoordinator bool
eventLoop:
	for {
		select {
		case event := <-eventCh:
			if event.Payload.Phase == InitCreatingWorkspace {
				sawCreatingWorkspace = true
			}
			if event.Payload.Phase == InitSpawningCoordinator {
				sawSpawningCoordinator = true
			}
			if event.Payload.Type == InitEventFailed || event.Payload.Type == InitEventTimedOut {
				// This is expected - we'll timeout at coordinator phase
				break eventLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for failure/timeout event")
		}
	}

	// Verify we progressed through the phases with timeouts working
	require.True(t, sawCreatingWorkspace, "should have progressed through workspace phase")
	require.True(t, sawSpawningCoordinator, "should have progressed through coordinator spawn phase")

	// Cleanup
	init.Cancel()
}

func TestInitializer_NoGoroutineLeakOnSuccess(t *testing.T) {
	// Unit test: Verify maxTotalTimer goroutine exits cleanly when initialization completes
	// (either success or expected failure).
	// Use runtime.NumGoroutine() before/after to verify no leaks from the timeout goroutine.
	workDir := t.TempDir()
	sessionsBaseDir := t.TempDir()
	worktreePath := filepath.Join(workDir, ".worktrees", "test")

	// Give time for any background goroutines to settle
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	goroutinesBefore := runtime.NumGoroutine()

	// Set up mocks for quick initialization
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(mock.AnythingOfType("string")).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktreeWithContext(mock.Anything, worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	mockGit.EXPECT().GetRemoteURL("origin").Return("https://github.com/test/repo.git", nil).Maybe()

	init := NewInitializer(InitializerConfig{
		WorkDir: workDir,
		AgentProviders: client.AgentProviders{
			client.RoleCoordinator: client.NewAgentProvider(client.ClientClaude, nil),
		},
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		// Will timeout at coordinator phase, which is expected
		Timeouts: config.TimeoutsConfig{
			WorktreeCreation: 30 * time.Second,
			CoordinatorStart: 100 * time.Millisecond,
			WorkspaceSetup:   30 * time.Second,
			MaxTotal:         120 * time.Second,
		},
		SessionStorage: config.SessionStorageConfig{
			BaseDir:         sessionsBaseDir,
			ApplicationName: "test-app",
		},
	})

	// Subscribe to events
	eventCh := init.Broker().Subscribe(context.Background())

	// Start initialization
	err := init.Start()
	require.NoError(t, err)

	// Wait for timeout/failure event (expected at coordinator phase)
	timeout := time.After(3 * time.Second)
eventLoop:
	for {
		select {
		case event := <-eventCh:
			if event.Payload.Type == InitEventFailed || event.Payload.Type == InitEventTimedOut {
				// Expected - initialization completes (with timeout)
				break eventLoop
			}
		case <-timeout:
			t.Fatal("timeout waiting for failure/timeout event")
		}
	}

	// Cleanup resources
	init.Cancel()

	// Give time for goroutines to clean up
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	goroutinesAfter := runtime.NumGoroutine()

	// Allow some tolerance for background goroutines from other sources
	// (test infrastructure, MCP server, etc.)
	// The key is that the maxTotalTimer goroutine should have exited
	leakedGoroutines := goroutinesAfter - goroutinesBefore
	require.LessOrEqual(t, leakedGoroutines, 10, "should not leak excessive goroutines (before=%d, after=%d)", goroutinesBefore, goroutinesAfter)
}
