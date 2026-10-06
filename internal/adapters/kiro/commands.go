package kiro

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func commandsDir(cfg *config.Config) string {
	return emit.OutputCommandsDir(cfg, target, emit.OutputSubDir(cfg, target, "prompts", defaultCommandsDir))
}

func emitCommands(sess *emit.Session, commands []spec.Entry, dir string, dryRun bool) error {
	for _, command := range commands {
		path := filepath.Join(dir, command.Name+".md")
		body := command.Body
		if body != "" {
			body = emit.HeaderBlock(emit.FormatMarkdown) + body
		}
		if err := sess.WriteFile(path, body, dryRun); err != nil {
			return err
		}
	}
	return nil
}
