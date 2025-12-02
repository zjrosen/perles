// Package markdown provides styled markdown rendering for the TUI.
package markdown

import (
	"github.com/charmbracelet/glamour"
)

// noMarginStyle is a JSON style that removes document margins.
// It inherits from auto (dark/light detection) but overrides margin to 0.
const noMarginStyle = `{
	"document": {
		"margin": 0,
		"block_prefix": "",
		"block_suffix": ""
	}
}`

// Renderer wraps glamour with perles-specific configuration.
type Renderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// New creates a markdown renderer with the given width.
func New(width int) (*Renderer, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithStylesFromJSONBytes([]byte(noMarginStyle)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	return &Renderer{renderer: r, width: width}, nil
}

// Width returns the configured word wrap width.
func (r *Renderer) Width() int {
	return r.width
}

// Render transforms markdown to styled terminal output.
func (r *Renderer) Render(markdown string) (string, error) {
	return r.renderer.Render(markdown)
}
