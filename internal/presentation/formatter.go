package presentation

import (
	"encoding/json"
	"io"
)

// Formatter handles output formatting
type Formatter struct {
	writer io.Writer
}

// NewFormatter creates a new formatter
func NewFormatter(writer io.Writer) *Formatter {
	return &Formatter{
		writer: writer,
	}
}

// FormatRegistrations formats a list of registrations as JSON
func (f *Formatter) FormatRegistrations(registrations []RegistrationDTO) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(registrations)
}

// FormatWorkflowResult formats a workflow creation result as JSON
func (f *Formatter) FormatWorkflowResult(result any) error {
	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
