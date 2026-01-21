package registry

import "errors"

// Builder errors
var (
	ErrEmptyNamespace = errors.New("registration namespace cannot be empty")
	ErrEmptyKey       = errors.New("registration key cannot be empty")
	ErrEmptyVersion   = errors.New("registration version cannot be empty")
	ErrEmptyChain     = errors.New("registration must have at least one chain item")
)

// Builder provides a fluent API for creating registrations
type Builder struct {
	namespace   string
	key         string
	version     string
	name        string
	description string
	template    string
	dag         *Chain
	labels      []string
}

// NewBuilder creates a new registration builder
func NewBuilder(namespace string) *Builder {
	return &Builder{
		namespace: namespace,
	}
}

// Key sets the registration key (unique identifier per type)
func (b *Builder) Key(k string) *Builder {
	b.key = k
	return b
}

// Version sets the registration version
func (b *Builder) Version(v string) *Builder {
	b.version = v
	return b
}

// Name sets the human-readable name
func (b *Builder) Name(n string) *Builder {
	b.name = n
	return b
}

// Description sets the description for AI agents
func (b *Builder) Description(d string) *Builder {
	b.description = d
	return b
}

// Template sets the template filename for the epic description
func (b *Builder) Template(t string) *Builder {
	b.template = t
	return b
}

// SetChain sets the workflow chain for the registration.
func (b *Builder) SetChain(chain *Chain) *Builder {
	b.dag = chain
	return b
}

// Labels sets the registration labels for filtering
func (b *Builder) Labels(labels ...string) *Builder {
	b.labels = labels
	return b
}

// Build creates the registration, validating required fields
func (b *Builder) Build() (*Registration, error) {
	if b.namespace == "" {
		return nil, ErrEmptyNamespace
	}
	if b.key == "" {
		return nil, ErrEmptyKey
	}
	if b.version == "" {
		return nil, ErrEmptyVersion
	}
	if b.dag == nil {
		return nil, ErrEmptyChain
	}

	return newRegistration(b.namespace, b.key, b.version, b.name, b.description, b.template, b.dag, b.labels), nil
}
