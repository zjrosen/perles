package cmd

import (
	"os"

	"github.com/spf13/cobra"

	appreg "github.com/zjrosen/perles/internal/application/registry"
	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/presentation"
)

var (
	workflowFeatureSlug string
	workflowKey         string
)

var workflowCreateCmd = &cobra.Command{
	Use:   "workflow:create",
	Short: "Create epic and tasks from workflow definition",
	Long: `Create a bd epic with all workflow tasks and dependencies from a workflow definition.

This command deterministically creates bd artifacts (epic, tasks, dependencies) from
a workflow registration. It replaces LLM-driven task creation with a single,
reliable command.

Required inputs:
  --feature (-f): Feature slug, e.g., "my-feature"
  --workflow (-w): Workflow key from registry, e.g., "research-proposal"

Prerequisites:
  - Spec file must exist at .spec/{feature}/spec.md
  - Workflow must exist in registry (use 'perles registry:list' to see available)

Output:
  JSON output includes epic info, workflow info, and tasks with resolved dependency IDs.

Examples:
  # Create epic and tasks for a feature using research proposal workflow
  perles workflow:create --feature my-feature --workflow research-proposal

  # Short flags
  perles workflow:create -f my-feature -w research-proposal

  # Parse specific fields with jq
  perles workflow:create -f my-feature -w research-proposal | jq '.tasks[].name'`,
	RunE: runWorkflowCreate,
}

func runWorkflowCreate(cmd *cobra.Command, args []string) error {
	// Validate required flags
	if workflowFeatureSlug == "" {
		return cmd.Help()
	}
	if workflowKey == "" {
		return cmd.Help()
	}

	// Create BeadsExecutor with working directory context
	executor := beads.NewRealExecutor("", "")

	// Create WorkflowCreator with dependencies
	creator := appreg.NewWorkflowCreator(registryService, executor)

	// Create the epic and tasks
	result, err := creator.Create(workflowFeatureSlug, workflowKey)
	if err != nil {
		return err
	}

	// Format output as JSON
	formatter := presentation.NewFormatter(os.Stdout)
	return formatter.FormatWorkflowResult(result)
}

func init() {
	workflowCreateCmd.Flags().StringVarP(&workflowFeatureSlug, "feature", "f", "", "Feature slug (required)")
	workflowCreateCmd.Flags().StringVarP(&workflowKey, "workflow", "w", "", "Workflow key from registry (required)")
	rootCmd.AddCommand(workflowCreateCmd)
}
