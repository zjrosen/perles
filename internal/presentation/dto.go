package presentation

import (
	"github.com/zjrosen/perles/internal/domain/registry"
)

// RegistrationDTO represents a workflow registration for presentation
type RegistrationDTO struct {
	Namespace   string    `json:"namespace"`
	Key         string    `json:"key"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Nodes       []NodeDTO `json:"nodes"`
	Labels      []string  `json:"labels"`
}

// NodeDTO represents a workflow node with dependency information
type NodeDTO struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Template   string   `json:"template"`
	Identifier string   `json:"identifier"`
	Inputs     []string `json:"inputs,omitempty"`
	Outputs    []string `json:"outputs,omitempty"`
	DependsOn  []string `json:"depends_on"` // always present, computed from inputs + after
}

// FromDomainNode converts a domain node to a DTO with computed dependencies.
func FromDomainNode(node *registry.Node, reg *registry.Registration, chain *registry.Chain) NodeDTO {
	// Extract artifact filenames for inputs
	inputs := make([]string, len(node.Inputs()))
	for i, a := range node.Inputs() {
		inputs[i] = a.Filename()
	}

	// Extract artifact filenames for outputs
	outputs := make([]string, len(node.Outputs()))
	for i, a := range node.Outputs() {
		outputs[i] = a.Filename()
	}

	// Compute depends_on from chain.DependenciesOf
	deps := chain.DependenciesOf(node.Key())
	dependsOn := make([]string, len(deps))
	for i, d := range deps {
		dependsOn[i] = d.Key()
	}

	return NodeDTO{
		Key:        node.Key(),
		Name:       node.Name(),
		Template:   node.Template(),
		Identifier: registry.BuildIdentifier(reg.Namespace(), reg.Key(), reg.Version(), node.Key()),
		Inputs:     inputs,
		Outputs:    outputs,
		DependsOn:  dependsOn,
	}
}

// FromDomainRegistration converts a domain registration to a DTO
func FromDomainRegistration(reg *registry.Registration) RegistrationDTO {
	// Build node DTOs with full dependency information
	dag := reg.DAG()
	nodes := make([]NodeDTO, 0)
	if dag != nil {
		for _, node := range dag.Nodes() {
			nodes = append(nodes, FromDomainNode(node, reg, dag))
		}
	}

	return RegistrationDTO{
		Namespace:   reg.Namespace(),
		Key:         reg.Key(),
		Version:     reg.Version(),
		Name:        reg.Name(),
		Description: reg.Description(),
		Nodes:       nodes,
		Labels:      reg.Labels(),
	}
}

// FromDomainRegistrations converts a slice of domain registrations to DTOs
func FromDomainRegistrations(regs []*registry.Registration) []RegistrationDTO {
	dtos := make([]RegistrationDTO, len(regs))
	for i, reg := range regs {
		dtos[i] = FromDomainRegistration(reg)
	}
	return dtos
}
