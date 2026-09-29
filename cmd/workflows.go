package cmd

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/zjrosen/perles/internal/orchestration/workflow"
	appreg "github.com/zjrosen/perles/internal/registry/application"
	"github.com/zjrosen/perles/internal/registry/domain"
)

// dashboardWorkflowNamespace is the registry namespace listed by the
// dashboard's New Workflow dialog.
const dashboardWorkflowNamespace = "workflow"

var workflowsCmd = &cobra.Command{
	Use:   "workflows",
	Short: "List available workflow templates",
	Long: `List the workflow templates perles can run, in two sections:

Dashboard Workflows are the multi-agent templates offered by the dashboard's
New Workflow dialog: built-in templates, community workflows enabled in
orchestration.community_workflows, and your own templates in
~/.perles/workflows/<name>/template.yaml.

Chat Panel Workflows are the markdown workflows offered by the chat panel's
Workflows tab: built-in chat workflows and your own ~/.perles/workflows/*.md
files.`,
	RunE: runWorkflows,
}

func init() {
	rootCmd.AddCommand(workflowsCmd)
}

func runWorkflows(cmd *cobra.Command, args []string) error {
	// Dashboard workflows: the same registry service (and namespace) the
	// dashboard's New Workflow dialog reads.
	svc, err := newRegistryService(cfg.Orchestration)
	if err != nil {
		return fmt.Errorf("loading dashboard workflows: %w", err)
	}

	// Chat panel workflows: the markdown workflow registry, filtered the same
	// way the chat panel's Workflows tab filters it.
	chatRegistry, err := workflow.NewRegistryWithConfig(cfg.Orchestration)
	if err != nil {
		return fmt.Errorf("loading chat panel workflows: %w", err)
	}

	var b strings.Builder
	writeDashboardWorkflows(&b, svc.GetByNamespace(dashboardWorkflowNamespace), dashboardKey())
	b.WriteString("\n")
	writeChatPanelWorkflows(&b, chatRegistry.ListByTargetMode(workflow.TargetChat))

	_, err = io.WriteString(cmd.OutOrStdout(), b.String())
	return err
}

// dashboardKey returns the configured key that opens the dashboard.
func dashboardKey() string {
	if cfg.UI.Keybindings.Dashboard != "" {
		return cfg.UI.Keybindings.Dashboard
	}
	return "ctrl+o"
}

// writeDashboardWorkflows writes the Dashboard Workflows section.
func writeDashboardWorkflows(b *strings.Builder, regs []*registry.Registration, openDashboardKey string) {
	b.WriteString("Dashboard Workflows (dashboard New Workflow dialog):\n")

	hasCommunity := false
	if len(regs) == 0 {
		b.WriteString("  (none)\n")
	} else {
		keyWidth, sourceWidth := 0, 0
		for _, reg := range regs {
			keyWidth = max(keyWidth, len(reg.Key()))
			sourceWidth = max(sourceWidth, len(sourceLabel(reg.Source().String())))
			if reg.Source() == registry.SourceCommunity {
				hasCommunity = true
			}
		}
		for _, reg := range regs {
			fmt.Fprintf(b, "  %-*s  %-*s  %s\n", keyWidth, reg.Key(),
				sourceWidth, sourceLabel(reg.Source().String()),
				nameAndDescription(reg.Name(), reg.Description()))
		}
	}

	b.WriteString("\n")
	fmt.Fprintf(b, "  User templates:      %s\n",
		filepath.Join(appreg.UserRegistryBaseDir(), "workflows", "<name>", "template.yaml"))
	if !hasCommunity {
		b.WriteString("  Community workflows: none enabled (add IDs to orchestration.community_workflows)\n")
	}
	fmt.Fprintf(b, "  Start one:           press %s to open the dashboard, then n (New Workflow)\n", openDashboardKey)
}

// writeChatPanelWorkflows writes the Chat Panel Workflows section.
func writeChatPanelWorkflows(b *strings.Builder, workflows []workflow.Workflow) {
	b.WriteString("Chat Panel Workflows (chat panel Workflows tab):\n")

	if len(workflows) == 0 {
		b.WriteString("  (none)\n")
	} else {
		idWidth, sourceWidth := 0, 0
		for _, wf := range workflows {
			idWidth = max(idWidth, len(wf.ID))
			sourceWidth = max(sourceWidth, len(sourceLabel(wf.Source.String())))
		}
		for _, wf := range workflows {
			fmt.Fprintf(b, "  %-*s  %-*s  %s\n", idWidth, wf.ID,
				sourceWidth, sourceLabel(wf.Source.String()),
				nameAndDescription(wf.Name, wf.Description))
		}
	}

	b.WriteString("\n")
	fmt.Fprintf(b, "  User workflows:      %s\n", filepath.Join(workflow.UserWorkflowDir(), "*.md"))
	b.WriteString("  Start one:           press ctrl+w in kanban or search mode to open the chat panel,\n")
	b.WriteString("                       then ctrl+t (Workflows tab) and enter to run the selected workflow\n")
}

// sourceLabel formats a workflow source (built-in, community, user) as a label.
func sourceLabel(source string) string {
	return "[" + source + "]"
}

// nameAndDescription joins a workflow's display name and description,
// collapsing multi-line descriptions onto a single line.
func nameAndDescription(name, description string) string {
	description = strings.Join(strings.Fields(description), " ")
	if description == "" {
		return name
	}
	return name + " - " + description
}
