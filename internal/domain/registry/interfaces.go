package registry

// RegistryProvider defines read-only access to a registry of registrations.
// This interface enables dependency injection and facilitates testing by
// allowing mock implementations to be substituted for the concrete Registry.
type RegistryProvider interface {
	// List returns all registrations in the registry.
	List() []*Registration

	// GetByNamespace returns all registrations matching the specified namespace.
	GetByNamespace(namespace string) []*Registration

	// GetByKey returns a specific registration by namespace and key.
	// Returns ErrNotFound if no registration matches.
	GetByKey(namespace, key string) (*Registration, error)

	// GetByLabels returns registrations that have ALL specified labels (AND logic).
	// If no labels are provided, returns all registrations.
	GetByLabels(labels ...string) []*Registration

	// Labels returns all unique labels across all registrations, sorted alphabetically.
	Labels() []string
}

// Compile-time check that Registry implements RegistryProvider.
var _ RegistryProvider = (*Registry)(nil)
