package cmd

import (
	"fmt"

	"github.com/zjrosen/perles/internal/mode/playground"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var playgroundCmd = &cobra.Command{
	Use:   "playground",
	Short: "Interactive playground for testing vimtextarea",
	Long:  `Launch an interactive playground to test vim keybindings and text editing features.`,
	RunE:  runPlayground,
}

func init() {
	rootCmd.AddCommand(playgroundCmd)
}

func runPlayground(cmd *cobra.Command, args []string) error {
	model := playground.New()
	p := tea.NewProgram(
		&model,
		tea.WithAltScreen(),
	)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("running playground: %w", err)
	}
	return nil
}
