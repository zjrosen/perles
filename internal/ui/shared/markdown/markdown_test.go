package markdown

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// stripANSI removes ANSI escape codes from a string for easier testing.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func TestNew(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "unexpected error")
	require.NotNil(t, r, "expected non-nil renderer")
	require.Equal(t, 80, r.Width())
}

func TestRenderer_Width(t *testing.T) {
	tests := []int{40, 80, 120}
	for _, w := range tests {
		r, err := New(w, "")
		require.NoError(t, err, "New(%d) error", w)
		require.Equal(t, w, r.Width())
	}
}

func TestRenderer_Render_Heading(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("# Title\n\nContent")
	require.NoError(t, err, "Render error")

	require.Contains(t, result, "Title", "expected result to contain 'Title'")
	require.Contains(t, result, "Content", "expected result to contain 'Content'")
}

func TestRenderer_Render_CodeBlock(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("```go\nfunc main() {}\n```")
	require.NoError(t, err, "Render error")

	require.Contains(t, result, "func", "expected result to contain 'func'")
	require.Contains(t, result, "main", "expected result to contain 'main'")
}

func TestRenderer_Render_List(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("- Item 1\n- Item 2\n- Item 3")
	require.NoError(t, err, "Render error")

	// Strip ANSI codes for content checking since glamour inserts codes between characters
	stripped := stripANSI(result)
	require.Contains(t, stripped, "Item 1", "expected result to contain 'Item 1'")
	require.Contains(t, stripped, "Item 2", "expected result to contain 'Item 2'")
}

func TestRenderer_Render_Bold(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("This is **bold** text")
	require.NoError(t, err, "Render error")

	require.Contains(t, result, "bold", "expected result to contain 'bold'")
}

func TestRenderer_Render_EmptyString(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("")
	require.NoError(t, err, "Render error")

	// Empty input should produce minimal or empty output
	require.LessOrEqual(t, len(result), 10, "expected minimal output for empty string, got: %q", result)
}

func TestRenderer_Render_PlainText(t *testing.T) {
	r, err := New(80, "")
	require.NoError(t, err, "New error")

	result, err := r.Render("Just plain text without any markdown")
	require.NoError(t, err, "Render error")

	require.True(t, strings.Contains(result, "plain text"), "expected result to contain 'plain text'")
}
