package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zjrosen/perles/internal/app"
	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/ui/nobeads"
	"github.com/zjrosen/perles/internal/ui/outdated"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	version   = "dev"
	cfgFile   string
	cfg       config.Config
	debugFlag bool
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

	// Bind flags to viper
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
	viper.SetDefault("orchestration.client", defaults.Orchestration.Client)
	viper.SetDefault("orchestration.claude.model", defaults.Orchestration.Claude.Model)
	viper.SetDefault("orchestration.amp.model", defaults.Orchestration.Amp.Model)
	viper.SetDefault("orchestration.amp.mode", defaults.Orchestration.Amp.Mode)

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

	if err := config.ValidateViews(cfg.Views); err != nil {
		return fmt.Errorf("invalid view configuration: %w", err)
	}

	if err := config.ValidateOrchestration(cfg.Orchestration); err != nil {
		return fmt.Errorf("invalid orchestration configuration: %w", err)
	}

	// Use provided beads directory or current directory
	dbPath := cfg.BeadsDir
	if dbPath == "" {
		var err error
		dbPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}

	client, err := beads.NewClient(dbPath)
	if err != nil {
		// Show friendly TUI empty state instead of CLI error
		return runNoBeadsMode()
	}

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

	// Pass config to app with database and config paths (debug for log overlay)
	model := app.NewWithConfig(client, cfg, dbPath+"/.beads/beads.db", configFilePath, debug)
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err = p.Run()

	// Log shutdown (only in debug mode - log is initialized)
	if debug {
		if err != nil {
			log.Error(log.CatConfig, "Perles shutting down with error", "error", err)
		} else {
			log.Info(log.CatConfig, "Perles shutting down")
		}
	}

	// Clean up watcher resources
	if closeErr := model.Close(); closeErr != nil && err == nil {
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
