package registry

import (
	"errors"
	"sort"
)

// Registry errors
var (
	ErrNotFound        = errors.New("registration not found")
	ErrDuplicateKey    = errors.New("duplicate key for registration type")
	ErrNilRegistration = errors.New("registration cannot be nil")
)

// Registry holds all registrations
type Registry struct {
	registrations []*Registration
}

// NewRegistry creates a new empty registry
func NewRegistry() *Registry {
	return &Registry{
		registrations: make([]*Registration, 0),
	}
}

// Add adds a registration to the registry
func (r *Registry) Add(reg *Registration) error {
	if reg == nil {
		return ErrNilRegistration
	}

	// Check for duplicate namespace+key
	for _, existing := range r.registrations {
		if existing.Namespace() == reg.Namespace() && existing.Key() == reg.Key() {
			return ErrDuplicateKey
		}
	}

	r.registrations = append(r.registrations, reg)
	return nil
}

// List returns all registrations
func (r *Registry) List() []*Registration {
	return r.registrations
}

// GetByNamespace returns all registrations for a namespace
func (r *Registry) GetByNamespace(namespace string) []*Registration {
	result := make([]*Registration, 0)
	for _, reg := range r.registrations {
		if reg.Namespace() == namespace {
			result = append(result, reg)
		}
	}
	return result
}

// GetByKey returns a specific registration by namespace and key
func (r *Registry) GetByKey(namespace, key string) (*Registration, error) {
	for _, reg := range r.registrations {
		if reg.Namespace() == namespace && reg.Key() == key {
			return reg, nil
		}
	}
	return nil, ErrNotFound
}

// GetByLabels returns registrations that have ALL specified labels (AND logic)
func (r *Registry) GetByLabels(labels ...string) []*Registration {
	if len(labels) == 0 {
		return r.registrations
	}
	result := make([]*Registration, 0)
	for _, reg := range r.registrations {
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

// Labels returns all unique labels across all registrations, sorted alphabetically
func (r *Registry) Labels() []string {
	labelSet := make(map[string]bool)
	for _, reg := range r.registrations {
		for _, label := range reg.Labels() {
			labelSet[label] = true
		}
	}

	labels := make([]string, 0, len(labelSet))
	for label := range labelSet {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return labels
}
