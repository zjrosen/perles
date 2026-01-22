package registry

import "errors"

// ArgumentType defines the type of input for a workflow argument.
type ArgumentType string

const (
	// ArgumentTypeText is a single-line text input.
	ArgumentTypeText ArgumentType = "text"
	// ArgumentTypeNumber is a numeric input.
	ArgumentTypeNumber ArgumentType = "number"
	// ArgumentTypeTextarea is a multi-line text input.
	ArgumentTypeTextarea ArgumentType = "textarea"
	// ArgumentTypeSelect is a single-select dropdown.
	ArgumentTypeSelect ArgumentType = "select"
	// ArgumentTypeMultiSelect is a multi-select list.
	ArgumentTypeMultiSelect ArgumentType = "multi-select"
)

// IsValid returns true if the argument type is a known type.
func (t ArgumentType) IsValid() bool {
	switch t {
	case ArgumentTypeText, ArgumentTypeNumber, ArgumentTypeTextarea, ArgumentTypeSelect, ArgumentTypeMultiSelect:
		return true
	default:
		return false
	}
}

// RequiresOptions returns true if the argument type requires options to be set.
func (t ArgumentType) RequiresOptions() bool {
	return t == ArgumentTypeSelect || t == ArgumentTypeMultiSelect
}

// Argument errors
var (
	ErrArgumentEmptyKey     = errors.New("argument key cannot be empty")
	ErrArgumentEmptyLabel   = errors.New("argument label cannot be empty")
	ErrArgumentEmptyType    = errors.New("argument type cannot be empty")
	ErrArgumentInvalidType  = errors.New("argument type must be text, number, textarea, select, or multi-select")
	ErrArgumentEmptyOptions = errors.New("argument options cannot be empty for select/multi-select types")
)

// Argument represents a user-configurable parameter for a workflow template.
// Arguments are rendered as form fields in the TUI and made available
// in TemplateContext for template rendering.
type Argument struct {
	key          string       // Unique identifier (used in templates as {{.Args.key}})
	label        string       // Human-readable label for the form field
	description  string       // Help text/placeholder for the form field
	argType      ArgumentType // Input type: text, number, textarea, select, multi-select
	required     bool         // Whether the argument is required
	defaultValue string       // Default value (optional)
	options      []string     // Available choices for select/multi-select types
}

// NewArgument creates a new Argument with validation.
// For select/multi-select types, use NewArgumentWithOptions instead.
func NewArgument(key, label, description string, argType ArgumentType, required bool, defaultValue string) (*Argument, error) {
	return NewArgumentWithOptions(key, label, description, argType, required, defaultValue, nil)
}

// NewArgumentWithOptions creates a new Argument with options for select/multi-select types.
func NewArgumentWithOptions(key, label, description string, argType ArgumentType, required bool, defaultValue string, options []string) (*Argument, error) {
	if key == "" {
		return nil, ErrArgumentEmptyKey
	}
	if label == "" {
		return nil, ErrArgumentEmptyLabel
	}
	if argType == "" {
		return nil, ErrArgumentEmptyType
	}
	if !argType.IsValid() {
		return nil, ErrArgumentInvalidType
	}
	if argType.RequiresOptions() && len(options) == 0 {
		return nil, ErrArgumentEmptyOptions
	}

	return &Argument{
		key:          key,
		label:        label,
		description:  description,
		argType:      argType,
		required:     required,
		defaultValue: defaultValue,
		options:      options,
	}, nil
}

// Key returns the argument's unique identifier.
func (a *Argument) Key() string {
	return a.key
}

// Label returns the human-readable label.
func (a *Argument) Label() string {
	return a.label
}

// Description returns the help text/placeholder.
func (a *Argument) Description() string {
	return a.description
}

// Type returns the input type.
func (a *Argument) Type() ArgumentType {
	return a.argType
}

// Required returns whether the argument is required.
func (a *Argument) Required() bool {
	return a.required
}

// DefaultValue returns the default value.
func (a *Argument) DefaultValue() string {
	return a.defaultValue
}

// Options returns the available choices for select/multi-select types.
func (a *Argument) Options() []string {
	return a.options
}
