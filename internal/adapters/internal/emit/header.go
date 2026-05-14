package emit

import "github.com/chemaclass/agnostic-ai/internal/adapters/header"

// Format is the file-format hint adapters pass to Header / WithHeader.
// Re-exported from internal/adapters/header so adapters keep a single
// emit import while the cli package (which lives outside
// internal/adapters/) can still reuse the same definitions via the
// header package directly.
type Format = header.Format

const (
	FormatMarkdown = header.FormatMarkdown
	FormatTOML     = header.FormatTOML
	FormatYAML     = header.FormatYAML
	FormatShell    = header.FormatShell
	FormatJSON     = header.FormatJSON
)

// Header returns the comment line that marks a generated file in the
// given format, terminated by a newline. Returns "" for FormatJSON.
// Re-exports header.Line so adapters call one place.
func Header(format Format) string {
	return header.Line(format)
}

// WithHeader prepends Header(format) to content. For Markdown content
// with YAML frontmatter, the header is inserted right after the
// closing delimiter so the frontmatter parser stays valid. Re-exports
// header.With.
func WithHeader(content string, format Format) string {
	return header.With(content, format)
}
