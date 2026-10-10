package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/kiro"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func stageGlobalHookFiles(home, stage string, targets []string, have []spec.Entry, warn io.Writer) error {
	for _, target := range targets {
		g := globalTargets[target]
		if g.hooksDir == "" {
			continue
		}
		dir := g.path(home, g.hooksDir)
		files, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", dir, err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			name := strings.TrimSuffix(file.Name(), ".json")
			covered := false
			for _, hook := range have {
				if hook.Name == name && hook.EmitsTo(target) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			if err := stageGlobalKiroHook(filepath.Join(dir, file.Name()), name, stage, warn); err != nil {
				return err
			}
		}
	}
	return nil
}

func stageGlobalKiroHook(path, name, stage string, warn io.Writer) error {
	scratch, err := os.MkdirTemp("", "agnostic-ai-import-kiro-hook-")
	if err != nil {
		return fmt.Errorf("create hook staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	skip := func() error {
		_, err := fmt.Fprintf(warn, "warning: %s: skipped hook file; its specs would not write back the same file; split it into one hook spec per file or add it to the home by hand\n", path)
		return err
	}
	if spec.ValidateName(spec.KindHook, name) != nil {
		return skip()
	}
	if _, err := importKiroHookFile(path, scratch, map[string]bool{}); err != nil {
		return err
	}
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "import", Root: scratch, Sources: config.Sources{Hooks: "."}}})
	if err != nil {
		return err
	}
	if len(bundle.Hooks) != 1 {
		return skip()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	hook := bundle.Hooks[0]
	if hook.Name != name {
		return skip()
	}
	rendered, err := kiro.HookFile(hook)
	if err != nil {
		return err
	}
	var want, got any
	if json.Unmarshal(raw, &want) != nil || json.Unmarshal(rendered, &got) != nil || !sameSetting(want, got) {
		return skip()
	}
	dst := filepath.Join(stage, "hooks")
	if err := importMkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	return writeHookSpecFile(dst, name, hook.Meta)
}
