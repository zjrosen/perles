package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/zjrosen/perles/internal/beads/application"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
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
	"github.com/zjrosen/perles/internal/templates"

	// Register AI client providers (required for AgentProvider to work)
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/claude"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/codex"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/gemini"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/opencode"
)

// Silence unused import warning - config is used for type reference
var _ = config.Config{}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the control plane daemon",
	Long: `Run the control plane as a background daemon that exposes an HTTP API
for workflow management. Other tools can connect to manage workflows.

The daemon listens on the specified address (default: localhost:19999) and
provides REST endpoints for creating, starting, stopping, and monitoring
workflows.

Example:
  perles daemon                    # Start on default port
  perles daemon --addr :8080       # Start on port 8080
  perles daemon --addr /tmp/perles.sock  # Unix socket (future)`,
	RunE: runDaemon,
}

var (
	daemonAddr string
)

func init() {
	rootCmd.AddCommand(daemonCmd)

	daemonCmd.Flags().StringVar(&daemonAddr, "addr", "", "Address to listen on (overrides config)")
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
	// 1. -b flag (already in cfg.BeadsDir via viper binding)
	// 2. BEADS_DIR environment variable
	// 3. beads_dir config file setting (already in cfg.BeadsDir)
	// 4. Current working directory
	dbPath := cfg.BeadsDir
	if dbPath == "" {
		dbPath = os.Getenv("BEADS_DIR")
	}
	if dbPath == "" {
		dbPath = workDir
	}

	// Resolve full .beads path (handles redirect for worktrees, normalizes input)
	cfg.ResolvedBeadsDir = paths.ResolveBeadsDir(dbPath)
	log.Info(log.CatConfig, "resolved beads dir", "path", cfg.ResolvedBeadsDir)

	// Create beads executor for workflow creation
	var beadsExec application.IssueExecutor
	var workflowCreator *appreg.WorkflowCreator

	// Create registry service for template instructions with user-defined workflows
	// User workflows are loaded from ~/.perles/workflows/*/template.yaml
	registryService, err := appreg.NewRegistryService(
		templates.RegistryFS(),
		appreg.UserRegistryBaseDir(),
	)
	if err != nil {
		log.Error(log.CatConfig, "Failed to create registry service", "error", err)
		// Continue without registry service - prompts will work but without instructions
	}

	// Create beads executor for workflow creator
	beadsExec = infrabeads.NewBDExecutor(workDir, cfg.ResolvedBeadsDir)
	if registryService != nil {
		workflowCreator = appreg.NewWorkflowCreator(registryService, beadsExec)
	}

	// Create control plane
	cp, err := createDaemonControlPlane(&cfg, workDir)
	if err != nil {
		return fmt.Errorf("creating control plane: %w", err)
	}

	// Determine API server address
	// Priority: --addr flag > config api_port > auto-assign (port 0)
	addr := daemonAddr
	if addr == "" {
		port := cfg.Orchestration.APIPort
		addr = fmt.Sprintf("localhost:%d", port)
	}

	// Create API server
	server, err := api.NewServer(api.ServerConfig{
		Addr:            addr,
		ControlPlane:    cp,
		WorkflowCreator: workflowCreator,
		RegistryService: registryService,
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

func createDaemonControlPlane(cfg *config.Config, _ string) (controlplane.ControlPlane, error) {
	orchConfig := cfg.Orchestration

	// Create workflow registry
	workflowRegistry := workflow.NewRegistry()

	// Create components
	registry := controlplane.NewInMemoryRegistry()
	eventBus := controlplane.NewCrossWorkflowEventBus()

	sessionFactory := session.NewFactory(session.FactoryConfig{
		BaseDir: orchConfig.SessionStorage.BaseDir,
		// Note: GitExecutor not available in daemon mode without git context
	})

	soundService := sound.NewSystemSoundService(cfg.Sound.Events)

	supervisor, err := controlplane.NewSupervisor(controlplane.SupervisorConfig{
		AgentProviders:   orchConfig.AgentProviders(),
		WorkflowRegistry: workflowRegistry,
		SessionFactory:   sessionFactory,
		SoundService:     soundService,
		BeadsDir:         cfg.ResolvedBeadsDir,
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
