package qwen

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, c := range commands {
		for _, segment := range strings.Split(c.Name, ":") {
			if segment == "" || segment == "." || segment == ".." {
				return fmt.Errorf("qwen command %s: invalid namespace segment", c.Name)
			}
		}
		meta := emit.ResolveMeta(c.Meta, target)
		front := map[string]any{}
		if value, ok := meta["description"]; ok {
			front["description"] = value
		}
		var keys []string
		emit.MergeCustomTargetMeta(front, &keys, c.Meta, target, "description")
		body := emit.WithHeader(emit.Frontmatter(front)+"\n"+strings.TrimSpace(c.Body)+"\n", emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, strings.ReplaceAll(c.Name, ":", string(filepath.Separator))+".md"), body, dryRun); err != nil {
			return err
		}
	}
	return nil
}
