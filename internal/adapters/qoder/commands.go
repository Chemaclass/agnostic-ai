package qoder

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// commandFrontmatterKeys names the only frontmatter field the Qoder
// CLI's own field table documents for a command config file
// (docs.qoder.com/cli/commands): `description` (Required: Yes). That
// same page also documents `name` (Required: No), but states it
// "serves only as the display name in the TUI; the invocation name is
// always derived from the file path and is not affected by this
// field", so it carries no routing behavior this adapter needs to
// reproduce; see the package doc for why it is never written.
var commandFrontmatterKeys = []string{"description"}

// emitCommands writes one `<dir>/<name>.md` command file per spec.
func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		path := filepath.Join(dir, c.Name+".md")
		body := emit.WithHeader(commandFile(c), emit.FormatMarkdown)
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// commandFile renders a single command markdown file: `description`
// (falls back to the command's name when the spec has none, since the
// vendor documents the key as required rather than leaving it blank),
// plus any x-qoder custom key the author declares, then the body as
// the prompt.
func commandFile(e spec.Entry) string {
	resolved := emit.ResolveMeta(e.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = e.Name
	}
	front := map[string]any{"description": desc}
	keys := append([]string{}, commandFrontmatterKeys...)
	emit.MergeCustomTargetMeta(front, &keys, e.Meta, target, commandFrontmatterKeys...)
	var sb strings.Builder
	sb.WriteString(emit.FrontmatterOrdered(front, keys))
	sb.WriteString("\n")
	sb.WriteString(e.Body)
	return sb.String()
}
