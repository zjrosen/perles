package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArgumentType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		argType  ArgumentType
		expected bool
	}{
		{"text is valid", ArgumentTypeText, true},
		{"number is valid", ArgumentTypeNumber, true},
		{"textarea is valid", ArgumentTypeTextarea, true},
		{"select is valid", ArgumentTypeSelect, true},
		{"multi-select is valid", ArgumentTypeMultiSelect, true},
		{"empty is invalid", ArgumentType(""), false},
		{"unknown is invalid", ArgumentType("unknown"), false},
		{"checkbox is invalid", ArgumentType("checkbox"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.argType.IsValid())
		})
	}
}

func TestArgumentType_RequiresOptions(t *testing.T) {
	tests := []struct {
		name     string
		argType  ArgumentType
		expected bool
	}{
		{"text does not require options", ArgumentTypeText, false},
		{"number does not require options", ArgumentTypeNumber, false},
		{"textarea does not require options", ArgumentTypeTextarea, false},
		{"select requires options", ArgumentTypeSelect, true},
		{"multi-select requires options", ArgumentTypeMultiSelect, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.argType.RequiresOptions())
		})
	}
}

func TestNewArgument_Success(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		label        string
		description  string
		argType      ArgumentType
		required     bool
		defaultValue string
	}{
		{
			name:         "text argument",
			key:          "feature_name",
			label:        "Feature Name",
			description:  "Name of the feature to implement",
			argType:      ArgumentTypeText,
			required:     true,
			defaultValue: "",
		},
		{
			name:         "number argument with default",
			key:          "worker_count",
			label:        "Worker Count",
			description:  "Number of workers to spawn",
			argType:      ArgumentTypeNumber,
			required:     false,
			defaultValue: "4",
		},
		{
			name:         "textarea argument",
			key:          "context",
			label:        "Additional Context",
			description:  "Provide any additional context for the workflow",
			argType:      ArgumentTypeTextarea,
			required:     false,
			defaultValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, err := NewArgument(tt.key, tt.label, tt.description, tt.argType, tt.required, tt.defaultValue)
			require.NoError(t, err)
			require.NotNil(t, arg)

			assert.Equal(t, tt.key, arg.Key())
			assert.Equal(t, tt.label, arg.Label())
			assert.Equal(t, tt.description, arg.Description())
			assert.Equal(t, tt.argType, arg.Type())
			assert.Equal(t, tt.required, arg.Required())
			assert.Equal(t, tt.defaultValue, arg.DefaultValue())
		})
	}
}

func TestNewArgument_Validation(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		label        string
		description  string
		argType      ArgumentType
		required     bool
		defaultValue string
		expectedErr  error
	}{
		{
			name:        "empty key",
			key:         "",
			label:       "Label",
			argType:     ArgumentTypeText,
			expectedErr: ErrArgumentEmptyKey,
		},
		{
			name:        "empty label",
			key:         "key",
			label:       "",
			argType:     ArgumentTypeText,
			expectedErr: ErrArgumentEmptyLabel,
		},
		{
			name:        "empty type",
			key:         "key",
			label:       "Label",
			argType:     ArgumentType(""),
			expectedErr: ErrArgumentEmptyType,
		},
		{
			name:        "invalid type",
			key:         "key",
			label:       "Label",
			argType:     ArgumentType("checkbox"),
			expectedErr: ErrArgumentInvalidType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, err := NewArgument(tt.key, tt.label, tt.description, tt.argType, tt.required, tt.defaultValue)
			require.Error(t, err)
			require.Nil(t, arg)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestNewArgumentWithOptions_Select(t *testing.T) {
	options := []string{"option1", "option2", "option3"}

	arg, err := NewArgumentWithOptions(
		"environment",
		"Environment",
		"Select deployment environment",
		ArgumentTypeSelect,
		true,
		"option2",
		options,
	)
	require.NoError(t, err)
	require.NotNil(t, arg)

	assert.Equal(t, "environment", arg.Key())
	assert.Equal(t, "Environment", arg.Label())
	assert.Equal(t, ArgumentTypeSelect, arg.Type())
	assert.Equal(t, "option2", arg.DefaultValue())
	assert.Equal(t, options, arg.Options())
}

func TestNewArgumentWithOptions_MultiSelect(t *testing.T) {
	options := []string{"feature1", "feature2", "feature3"}

	arg, err := NewArgumentWithOptions(
		"features",
		"Features",
		"Select features to enable",
		ArgumentTypeMultiSelect,
		false,
		"",
		options,
	)
	require.NoError(t, err)
	require.NotNil(t, arg)

	assert.Equal(t, ArgumentTypeMultiSelect, arg.Type())
	assert.Equal(t, options, arg.Options())
}

func TestNewArgumentWithOptions_SelectWithoutOptions_Fails(t *testing.T) {
	arg, err := NewArgumentWithOptions(
		"env",
		"Environment",
		"Select environment",
		ArgumentTypeSelect,
		true,
		"",
		nil,
	)
	require.Error(t, err)
	require.Nil(t, arg)
	assert.ErrorIs(t, err, ErrArgumentEmptyOptions)
}

func TestNewArgumentWithOptions_MultiSelectWithoutOptions_Fails(t *testing.T) {
	arg, err := NewArgumentWithOptions(
		"features",
		"Features",
		"Select features",
		ArgumentTypeMultiSelect,
		false,
		"",
		[]string{},
	)
	require.Error(t, err)
	require.Nil(t, arg)
	assert.ErrorIs(t, err, ErrArgumentEmptyOptions)
}
