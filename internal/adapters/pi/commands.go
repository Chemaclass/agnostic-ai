package pi

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitCommands(sess *emit.Session, entries []spec.Entry, dir string, dryRun bool) error {
	for _, entry := range entries {
		if strings.ContainsAny(entry.Name, "/\\") {
			return fmt.Errorf("pi: command %q must be a direct prompt file name", entry.Name)
		}
		resolved := emit.ResolveMeta(entry.Meta, target)
		meta := map[string]any{}
		for _, key := range []string{"description", "argument-hint"} {
			if value, ok := resolved[key]; ok {
				meta[key] = value
			}
		}
		body := emit.Frontmatter(meta) + "\n" + entry.Body + "\n"
		if err := sess.WriteFile(filepath.Join(dir, entry.Name+".md"), emit.WithHeader(body, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	return nil
}
