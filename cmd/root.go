package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"perles/internal/app"
	"perles/internal/beads"
	"perles/internal/config"

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
	version = "dev"
	cfgFile string
	cfg     config.Config
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
	rootCmd.Flags().StringP("path", "p", "",
		"path to beads database directory")
	rootCmd.Flags().Bool("no-auto-refresh", false,
		"disable automatic board refresh when database changes")

	// Bind flags to viper
	_ = viper.BindPFlag("path", rootCmd.Flags().Lookup("path"))
}

func initConfig() {
	defaults := config.Defaults()
	viper.SetDefault("auto_refresh", defaults.AutoRefresh)
	viper.SetDefault("auto_refresh_debounce", defaults.AutoRefreshDebounce)
	viper.SetDefault("ui.show_counts", defaults.UI.ShowCounts)
	viper.SetDefault("theme.highlight", defaults.Theme.Highlight)
	viper.SetDefault("theme.subtle", defaults.Theme.Subtle)
	viper.SetDefault("theme.error", defaults.Theme.Error)
	viper.SetDefault("theme.success", defaults.Theme.Success)

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
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			defaultPath := ".perles/config.yaml"
			if writeErr := config.WriteDefaultConfig(defaultPath); writeErr == nil {
				viper.SetConfigFile(defaultPath)
				_ = viper.ReadInConfig()
			}
			// If write fails, just continue with defaults (no config file)
		}
	}

	_ = viper.Unmarshal(&cfg)
}

func runApp(cmd *cobra.Command, args []string) error {
	if err := config.ValidateViews(cfg.Views); err != nil {
		return fmt.Errorf("invalid view configuration: %w", err)
	}

	// Use provided path or current directory
	dbPath := cfg.Path
	if dbPath == "" {
		var err error
		dbPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
	}

	client, err := beads.NewClient(dbPath)
	if err != nil {
		return fmt.Errorf("connecting to beads: %w\nRun 'bd init' to initialize beads", err)
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

	// Pass config to app with database and config paths
	model := app.NewWithConfig(client, cfg, dbPath+"/.beads/beads.db", configFilePath)
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err = p.Run()

	// Clean up watcher resources
	if closeErr := model.Close(); closeErr != nil && err == nil {
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
