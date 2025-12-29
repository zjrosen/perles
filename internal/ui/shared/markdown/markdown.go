// Package markdown provides styled markdown rendering for the TUI.
package markdown

import (
	"github.com/charmbracelet/glamour"
)

// noMarginStyle is a JSON style that removes document margins.
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

// New creates a markdown renderer with the given width and style.
// style should be "dark" or "light". Defaults to "dark" if empty.
// Use DarkStyle instead of WithAutoStyle() to avoid terminal OSC queries.
// WithAutoStyle() creates a new lipgloss renderer that detects light/dark
// background by querying the terminal, which causes escape sequence responses
// to leak into the input stream.
func New(width int, style string) (*Renderer, error) {
	if style == "" {
		style = "dark"
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
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
