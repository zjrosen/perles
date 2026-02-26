package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/zjrosen/perles/communityworkflows"
	"github.com/zjrosen/perles/internal/app"
	beads "github.com/zjrosen/perles/internal/beads/domain"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/bql"
	"github.com/zjrosen/perles/internal/cachemanager"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/keys"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/paths"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/templates"
	"github.com/zjrosen/perles/internal/ui/nobeads"
	"github.com/zjrosen/perles/internal/ui/outdated"
	"github.com/zjrosen/perles/internal/ui/serverdown"
)

func init() {
	// Force lipgloss/termenv to query terminal background color BEFORE
	// any Bubble Tea program starts. This prevents the terminal's OSC 11
	// response from racing with Bubble Tea's input loop and appearing as
	// garbage text in input fields.
	//
	// See: https://github.com/charmbracelet/bubbletea/issues/1036
	_ = lipgloss.HasDarkBackground()
}

var (
	version         = "dev"
	cfgFile         string
	cfg             config.Config
	debugFlag       bool
	apiPortFlag     int
	registryService *appreg.RegistryService
)

var rootCmd = &cobra.Command{
	Use:     "perles",
	Short:   "A terminal ui for beads issue tracking",
	Long:    `A terminal user interface for viewing and managing beads issues in a kanban-style board with BQL support.`,
	Version: version,
	RunE:    runApp,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "",
		"config file (default: ~/.config/perles/config.yaml)")
	rootCmd.Flags().StringP("beads-dir", "b", "",
		"path to beads database directory")
	rootCmd.Flags().StringP("markdown-style", "", "",
		"markdown rendering style: \"dark\" (default) or \"light\"")
	rootCmd.PersistentFlags().BoolVarP(&debugFlag, "debug", "d", false,
		"enable debug mode with logging (also: PERLES_DEBUG=1)")
	rootCmd.Flags().IntVarP(&apiPortFlag, "port", "p", 0,
		"API server port (0 = auto-assign, overrides config)")

	_ = viper.BindPFlag("beads_dir", rootCmd.Flags().Lookup("beads-dir"))
	_ = viper.BindPFlag("ui.markdown_style", rootCmd.Flags().Lookup("markdown-style"))
}

func initConfig() {
	defaults := config.Defaults()
	viper.SetDefault("auto_refresh", defaults.AutoRefresh)
	viper.SetDefault("ui.show_counts", defaults.UI.ShowCounts)
	viper.SetDefault("ui.markdown_style", defaults.UI.MarkdownStyle)
	viper.SetDefault("theme.preset", defaults.Theme.Preset)

	// Orchestration defaults
	viper.SetDefault("orchestration.client", defaults.Orchestration.CoordinatorClient)
	viper.SetDefault("orchestration.coordinator_client", defaults.Orchestration.CoordinatorClient)
	viper.SetDefault("orchestration.worker_client", defaults.Orchestration.WorkerClient)
	viper.SetDefault("orchestration.claude.model", defaults.Orchestration.Claude.Model)
	viper.SetDefault("orchestration.amp.model", defaults.Orchestration.Amp.Model)
	viper.SetDefault("orchestration.amp.mode", defaults.Orchestration.Amp.Mode)

	// Sound defaults
	viper.SetDefault("sound.events", defaults.Sound.Events)

	// Keybinding defaults
	viper.SetDefault("ui.keybindings.search", "ctrl+space")
	viper.SetDefault("ui.keybindings.dashboard", "ctrl+o")

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Config lookup order:
		// 1. .perles/config.yaml (current directory)
		// 2. ~/.config/perles/config.yaml (user config)
		if _, err := os.Stat(".perles/config.yaml"); err == nil {
			viper.SetConfigFile(".perles/config.yaml")
		} else {
			home, _ := os.UserHomeDir()
			viper.AddConfigPath(filepath.Join(home, ".config", "perles"))
			viper.SetConfigName("config")
			viper.SetConfigType("yaml")
		}
	}

	if err := viper.ReadInConfig(); err != nil {
		// No config file found anywhere - create default at .perles/config.yaml
		var configNotFound viper.ConfigFileNotFoundError
		if errors.As(err, &configNotFound) {
			defaultPath := ".perles/config.yaml"
			if writeErr := config.WriteDefaultConfig(defaultPath); writeErr == nil {
				viper.SetConfigFile(defaultPath)
				_ = viper.ReadInConfig()
				log.Info(log.CatConfig, "Config loaded", "path", defaultPath)
			}
			// If write fails, just continue with defaults (no config file)
		}
	} else {
		log.Info(log.CatConfig, "Config loaded", "path", viper.ConfigFileUsed())
	}

	_ = viper.Unmarshal(&cfg)
}

func initServices() {
	// Initialize registry service with embedded templates, community, and user-defined workflows
	// templates.RegistryFS() contains template.yaml, workflow templates, and coordinator instructions
	// Community workflows are loaded from communityworkflows.RegistryFS(), filtered by config
	// User workflows are loaded from ~/.perles/workflows/*/template.yaml
	var communitySource *appreg.CommunitySource
	if len(cfg.Orchestration.CommunityWorkflows) > 0 {
		communitySource = &appreg.CommunitySource{
			FS:         communityworkflows.RegistryFS(),
			EnabledIDs: cfg.Orchestration.CommunityWorkflows,
		}
	}

	var err error
	registryService, err = appreg.NewRegistryService(
		templates.RegistryFS(),
		communitySource,
		appreg.UserRegistryBaseDir(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error initializing registry service:", err)
		os.Exit(1)
	}
}

func runApp(cmd *cobra.Command, args []string) error {
	// Initialize logging if debug mode enabled (via flag or env var)
	debug := os.Getenv("PERLES_DEBUG") != "" || debugFlag
	if debug {
		logPath := os.Getenv("PERLES_LOG")
		if logPath == "" {
			logPath = "debug.log"
		}

		cleanup, err := log.InitWithTeaLog(logPath, "perles")
		if err != nil {
			return fmt.Errorf("initializing logging: %w", err)
		}
		defer cleanup()

		// Log application startup
		log.Info(log.CatConfig, "Perles starting", "version", version, "debug", true, "logPath", logPath)
	}

	// Initialize registry service after logging so debug output is captured
	initServices()

	if err := config.ValidateViews(cfg.Views); err != nil {
		return fmt.Errorf("invalid view configuration: %w", err)
	}

	if err := config.ValidateOrchestration(cfg.Orchestration); err != nil {
		return fmt.Errorf("invalid orchestration configuration: %w", err)
	}

	if err := config.ValidateSound(cfg.Sound); err != nil {
		return fmt.Errorf("invalid sound configuration: %w", err)
	}

	// Apply --port flag override (takes precedence over config)
	if apiPortFlag != 0 {
		cfg.Orchestration.APIPort = apiPortFlag
	}

	// Validate keybindings before applying
	if err := config.ValidateKeybindings(cfg.UI.Keybindings); err != nil {
		return fmt.Errorf("invalid keybindings configuration: %w", err)
	}

	// Validate user-defined actions
	if err := config.ValidateActions(cfg.UI.Actions); err != nil {
		return fmt.Errorf("invalid actions configuration: %w", err)
	}

	// Apply keybinding overrides from config
	keys.ApplyConfig(cfg.UI.Keybindings.Search, cfg.UI.Keybindings.Dashboard)

	// Working directory is always the current directory (where perles was invoked)
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Resolution priority for beads directory:
	// 1. -b flag (explicitly provided on command line)
	// 2. BEADS_DIR environment variable
	// 3. beads_dir config file setting
	// 4. Current working directory
	var dbPath string
	if cmd.Flags().Changed("beads-dir") {
		// -b flag explicitly provided on command line
		dbPath, _ = cmd.Flags().GetString("beads-dir")
	} else if envDir := os.Getenv("BEADS_DIR"); envDir != "" {
		// BEADS_DIR environment variable
		dbPath = envDir
	} else if cfg.BeadsDir != "" {
		// beads_dir from config file
		dbPath = cfg.BeadsDir
	} else {
		// Default to working directory
		dbPath = workDir
	}

	// Resolve full .beads path (handles redirect for worktrees, normalizes input)
	cfg.ResolvedBeadsDir = paths.ResolveBeadsDir(dbPath)
	log.Info(log.CatConfig, "resolved beads dir", "path", cfg.ResolvedBeadsDir)

	client, err := infrabeads.NewClient(cfg.ResolvedBeadsDir)
	if err != nil {
		// If metadata says dolt+server, the failure is a connection issue, not a missing .beads dir.
		if meta, metaErr := infrabeads.LoadMetadata(cfg.ResolvedBeadsDir); metaErr == nil && meta.IsDoltServer() {
			return serverNotStarted(meta.GetDoltServerHost(), meta.GetDoltServerPortWithDir(cfg.ResolvedBeadsDir))
		}
		return runNoBeadsMode()
	}
	defer func() { _ = client.Close() }()

	// Version check - query bd_version from database metadata table
	currentVersion, err := client.Version()
	if err != nil {
		// Very old database without bd_version metadata - show outdated view
		log.Debug(log.CatBeads, "Version check failed", "error", err)
		return runOutdatedMode("unknown", beads.MinBeadsVersion)
	}

	log.Debug(log.CatBeads, "Beads Database Version", "version", currentVersion, "minRequiredVersion", beads.MinBeadsVersion)
	if err := beads.CheckVersion(currentVersion); err != nil {
		return runOutdatedMode(currentVersion, beads.MinBeadsVersion)
	}

	// Handle --no-auto-refresh flag (negated logic)
	if noAutoRefresh, _ := cmd.Flags().GetBool("no-auto-refresh"); noAutoRefresh {
		cfg.AutoRefresh = false
	}

	// Store the config file path for saving column changes
	configFilePath := viper.ConfigFileUsed()
	if configFilePath == "" {
		// No config file was loaded, default to .perles/config.yaml
		configFilePath = ".perles/config.yaml"
	}

	// Initialize BQL cache managers
	bqlCache := cachemanager.NewInMemoryCacheManager[string, []beads.Issue](
		"bql-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)
	depGraphCache := cachemanager.NewInMemoryCacheManager[string, *bql.DependencyGraph](
		"bql-dep-cache",
		cachemanager.DefaultExpiration,
		cachemanager.DefaultCleanupInterval,
	)

	// Pass config to app with database and config paths (debug for log overlay)
	model, err := app.NewWithConfig(
		client,
		cfg,
		bqlCache,
		depGraphCache,
		client.DBPath(),
		configFilePath,
		workDir,
		debug,
		registryService,
	)
	if err != nil {
		return fmt.Errorf("initializing application: %w", err)
	}
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)

	finalModel, err := p.Run()

	// Log shutdown (only in debug mode - log is initialized)
	if debug {
		if err != nil {
			log.Error(log.CatConfig, "Perles shutting down with error", "error", err)
		} else {
			log.Info(log.CatConfig, "Perles shutting down")
		}
	}

	// Clean up watcher resources using the final model state (which has controlPlane if dashboard was used)
	appModel := finalModel.(app.Model)
	if closeErr := appModel.Close(); closeErr != nil && err == nil {
		if debug {
			log.Error(log.CatConfig, "Error during cleanup", "error", closeErr)
		}
		err = closeErr
	}

	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

// SetVersion sets the version string (called from main with ldflags)
func SetVersion(v string) {
	version = v
	rootCmd.Version = v
}

// runNoBeadsMode launches the TUI in "no database" mode, showing a friendly
// empty state view when no .beads directory is found.
func runNoBeadsMode() error {
	model := nobeads.New()
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}

// serverNotStarted launches the TUI showing the Dolt server unreachable screen.
func serverNotStarted(host string, port int) error {
	model := serverdown.NewUnreachable(host, port)
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}

// runOutdatedMode launches the TUI showing the version upgrade screen.
func runOutdatedMode(currentVersion, requiredVersion string) error {
	model := outdated.New(currentVersion, requiredVersion)
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("running program: %w", err)
	}
	return nil
}
