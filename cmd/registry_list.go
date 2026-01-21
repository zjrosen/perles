package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/zjrosen/perles/internal/domain/registry"
	"github.com/zjrosen/perles/internal/presentation"
)

var (
	regNamespace string
	regLabels    []string
)

var registryListCmd = &cobra.Command{
	Use:   "registry:list",
	Short: "List all registered workflow types",
	Long: `List all registered workflow types and their versions as JSON.

Displays all available workflow registrations including their chain steps.
Use --namespace to filter registrations by namespace.
Use --label to filter by labels (repeatable, AND logic).

Examples:
  # List all registrations
  perles registry:list

  # Filter by namespace
  perles registry:list --namespace spec-workflow
  perles registry:list -n spec-workflow

  # Filter by single label
  perles registry:list --label lang:go
  perles registry:list -l lang:go

  # Filter by multiple labels (AND logic - must match ALL)
  perles registry:list -l lang:go -l category:guidelines

  # Combine namespace and label filters
  perles registry:list --namespace spec-workflow --label lang:go

  # Parse specific fields with jq
  perles registry:list | jq '.[].namespace'
  perles registry:list | jq '.[].labels'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var registrations []*registry.Registration

		hasNamespace := cmd.Flags().Changed("namespace")
		hasLabels := len(regLabels) > 0

		switch {
		case hasNamespace && hasLabels:
			// Combined filter: namespace first, then labels
			byNamespace := registryService.GetByNamespace(regNamespace)
			registrations = filterByLabels(byNamespace, regLabels)
		case hasNamespace:
			registrations = registryService.GetByNamespace(regNamespace)
		case hasLabels:
			registrations = registryService.GetByLabels(regLabels...)
		default:
			registrations = registryService.List()
		}

		formatter := presentation.NewFormatter(os.Stdout)
		dtos := presentation.FromDomainRegistrations(registrations)

		return formatter.FormatRegistrations(dtos)
	},
}

func init() {
	registryListCmd.Flags().StringVarP(&regNamespace, "namespace", "n", "", "Filter by registration namespace (e.g., spec-workflow)")
	registryListCmd.Flags().StringArrayVarP(&regLabels, "label", "l", nil, "Filter by label (can be repeated, e.g., --label lang:go)")
	rootCmd.AddCommand(registryListCmd)
}

// filterByLabels filters registrations by labels (AND logic)
func filterByLabels(regs []*registry.Registration, labels []string) []*registry.Registration {
	result := make([]*registry.Registration, 0)
	for _, reg := range regs {
		if hasAllLabels(reg.Labels(), labels) {
			result = append(result, reg)
		}
	}
	return result
}

// hasAllLabels checks if regLabels contains all targetLabels
func hasAllLabels(regLabels, targetLabels []string) bool {
	labelSet := make(map[string]bool)
	for _, l := range regLabels {
		labelSet[l] = true
	}
	for _, target := range targetLabels {
		if !labelSet[target] {
			return false
		}
	}
	return true
}
