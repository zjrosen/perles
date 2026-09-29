package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zjrosen/perles/communityworkflows"
	"github.com/zjrosen/perles/frontend"
	"github.com/zjrosen/perles/internal/config"
	appgit "github.com/zjrosen/perles/internal/git/application"
	infragit "github.com/zjrosen/perles/internal/git/infrastructure"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/controlplane/api"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/paths"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/sound"
	taskpkg "github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/templates"

	// Register AI client providers (required for AgentProvider to work)
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/claude"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/codex"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/cursor"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/gemini"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/opencode"
)

// Silence unused import warning — config is used for type reference in daemonPort/runDaemon
var _ = config.Config{}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the control plane daemon",
	Long: `Run the control plane as a background daemon that exposes an HTTP API
for workflow management. Other tools can connect to manage workflows.

The daemon listens on localhost with the specified port and provides REST
endpoints for creating, starting, stopping, and monitoring workflows.

Example:
  perles daemon                # Start on auto-assigned port
  perles daemon --port 8080    # Start on port 8080
  perles daemon -p 19999       # Start on port 19999`,
	RunE: runDaemon,
}

var (
	daemonPort int
)

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().IntVarP(&daemonPort, "port", "p", 0, "API server port (0 = auto-assign, overrides config)")
}

func runDaemon(_ *cobra.Command, _ []string) error {
	// Initialize logging if debug mode enabled (via flag or env var)
	debug := os.Getenv("PERLES_DEBUG") != "" || debugFlag
	if debug {
		logPath := os.Getenv("PERLES_LOG")
		if logPath == "" {
			logPath = "debug.log"
		}

		cleanup, err := log.InitWithTeaLog(logPath, "perles-daemon")
		if err != nil {
			return fmt.Errorf("initializing logging: %w", err)
		}
		defer cleanup()

		log.Info(log.CatConfig, "Perles daemon starting", "debug", true, "logPath", logPath)
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Resolution priority for beads directory (same as TUI):
	// 1. BEADS_DIR environment variable
	// 2. beads_dir config file setting
	// 3. Current working directory
	// Note: daemon doesn't have -b flag; use root command for explicit path
	var dbPath string
	if envDir := os.Getenv("BEADS_DIR"); envDir != "" {
		dbPath = envDir
	} else if cfg.BeadsDir != "" {
		dbPath = cfg.BeadsDir
	} else {
		dbPath = workDir
	}

	// Resolve full .beads path (handles redirect for worktrees, normalizes input)
	cfg.ResolvedBeadsDir = paths.ResolveBeadsDir(dbPath)
	log.Info(log.CatConfig, "resolved beads dir", "path", cfg.ResolvedBeadsDir)

	// Create backend using config-driven factory
	backend, err := newBackend(&cfg, workDir)
	if err != nil {
		return fmt.Errorf("creating backend: %w", err)
	}
	defer func() { _ = backend.Close() }()

	backendSvc := backend.Services()
	taskExec := backendSvc.TaskExecutor

	var workflowCreator *appreg.WorkflowCreator

	// Create registry service for template instructions with community and user-defined workflows
	// Community workflows are loaded from communityworkflows.RegistryFS(), filtered by config
	// User workflows are loaded from ~/.perles/workflows/*/template.yaml
	var communitySource *appreg.CommunitySource
	if len(cfg.Orchestration.CommunityWorkflows) > 0 {
		communitySource = &appreg.CommunitySource{
			FS:         communityworkflows.RegistryFS(),
			EnabledIDs: cfg.Orchestration.CommunityWorkflows,
		}
	}

	registryService, err := appreg.NewRegistryService(
		templates.RegistryFS(),
		communitySource,
		appreg.UserRegistryBaseDir(),
	)
	if err != nil {
		log.Error(log.CatConfig, "Failed to create registry service", "error", err)
		// Continue without registry service - prompts will work but without instructions
	}

	if registryService != nil {
		workflowCreator = appreg.NewWorkflowCreator(registryService, taskExec, cfg.Orchestration.Templates)
	}

	// Create control plane
	cp, err := createDaemonControlPlane(&cfg, workDir, taskExec)
	if err != nil {
		return fmt.Errorf("creating control plane: %w", err)
	}

	// Determine API server address
	// Priority: --port flag > config api_port > auto-assign (port 0)
	port := daemonPort
	if port == 0 {
		port = cfg.Orchestration.APIPort
	}
	addr := fmt.Sprintf("localhost:%d", port)

	// Create API server
	server, err := api.NewServer(api.ServerConfig{
		Addr:            addr,
		ControlPlane:    cp,
		WorkflowCreator: workflowCreator,
		RegistryService: registryService,
		FrontendFS:      frontend.DistFS(),
	})
	if err != nil {
		return fmt.Errorf("creating API server: %w", err)
	}

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	fmt.Printf("Perles daemon started on port %d\n", server.Port())
	fmt.Println("Press Ctrl+C to stop")

	// Wait for shutdown signal or error
	select {
	case sig := <-sigCh:
		fmt.Printf("\nReceived %s, shutting down...\n", sig)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	// Stop API server
	if err := server.Stop(shutdownCtx); err != nil {
		log.Error(log.CatOrch, "Error stopping API server", "error", err)
	}

	// Shutdown control plane (stops all workflows)
	if err := cp.Shutdown(shutdownCtx); err != nil {
		log.Error(log.CatOrch, "Error shutting down control plane", "error", err)
	}

	fmt.Println("Daemon stopped")
	return nil
}

func createDaemonControlPlane(cfg *config.Config, _ string, taskExec taskpkg.TaskExecutor) (controlplane.ControlPlane, error) {
	orchConfig := cfg.Orchestration

	// Create workflow registry
	workflowRegistry := workflow.NewRegistry()

	// Create components
	registry := controlplane.NewInMemoryRegistry()
	eventBus := controlplane.NewCrossWorkflowEventBus()

	sessionFactory := session.NewFactory(session.FactoryConfig{
		BaseDir: orchConfig.SessionStorage.BaseDir,
		// application_name (when set) overrides the derived name for all sessions
		ApplicationName: orchConfig.SessionStorage.ApplicationName,
		// Note: GitExecutor not available in daemon mode without git context,
		// so the derived name falls back to the workflow work directory basename
	})

	soundService := sound.NewSystemSoundService(cfg.Sound.Events)

	supervisor, err := controlplane.NewSupervisor(controlplane.SupervisorConfig{
		AgentProviders:   orchConfig.AgentProviders(),
		WorkflowRegistry: workflowRegistry,
		WorktreeTimeout:  orchConfig.Timeouts.WorktreeCreation,
		SessionFactory:   sessionFactory,
		SoundService:     soundService,
		BeadsDir:         cfg.ResolvedBeadsDir,
		TaskExecutor:     taskExec,
		GitExecutorFactory: func(path string) appgit.GitExecutor {
			return infragit.NewRealExecutor(path)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating supervisor: %w", err)
	}

	// Create health monitor
	healthMonitor := controlplane.NewHealthMonitor(controlplane.HealthMonitorConfig{
		Policy: controlplane.HealthPolicy{
			HeartbeatTimeout: 2 * time.Minute,
			ProgressTimeout:  10 * time.Minute,
			MaxRecoveries:    3,
			RecoveryBackoff:  30 * time.Second,
		},
		CheckInterval: 30 * time.Second,
		EventBus:      eventBus.Broker(),
	})

	// Create control plane
	cp, err := controlplane.NewControlPlane(controlplane.ControlPlaneConfig{
		Registry:      registry,
		Supervisor:    supervisor,
		EventBus:      eventBus,
		HealthMonitor: healthMonitor,
	})
	if err != nil {
		return nil, fmt.Errorf("creating control plane: %w", err)
	}

	// Start health monitor
	if err := healthMonitor.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("starting health monitor: %w", err)
	}

	return cp, nil
}
