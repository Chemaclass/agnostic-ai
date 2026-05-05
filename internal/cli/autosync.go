package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const autoSyncSpecName = "auto-sync"

const autoSyncSpecContent = `---
name: auto-sync
description: Run agnostic-ai sync when spec files change.
alwaysApply: true
---

When any file under the agnostic-ai source directories (agents, skills, rules, hooks, or MCPs) is added, edited, or removed during a session, run ` + "`agnostic-ai sync`" + ` to regenerate all target configs.
`

// handleAutoSync runs the one-time auto-sync setup when appropriate.
// Prompts on first sync (TTY only), or follows --auto-sync=yes|no.
// A nil return means no action was taken (already decided, non-TTY, dry-run).
func handleAutoSync(root, flag string, in io.Reader, out io.Writer) error {
	if err := validateAutoSyncFlag(flag); err != nil {
		return err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	enabled, err := resolveAutoSync(cfg, flag, in, out)
	if err != nil {
		return err
	}
	if enabled == nil {
		return nil
	}

	if err := config.PersistAutoSync(root, *enabled); err != nil {
		return fmt.Errorf("persist auto-sync: %w", err)
	}

	if *enabled {
		rulesDir := filepath.Join(root, cfg.Sources.Rules)
		if err := writeAutoSyncSpec(rulesDir); err != nil {
			return fmt.Errorf("write auto-sync spec: %w", err)
		}
		fmt.Fprintln(out, "→ auto-sync enabled: agents will run `agnostic-ai sync` when specs change")
	}

	return nil
}

// resolveAutoSync returns whether auto-sync should be enabled.
// Returns nil when no action is needed (already decided, or non-TTY without flag).
func resolveAutoSync(cfg *config.Config, flag string, in io.Reader, out io.Writer) (*bool, error) {
	yes, no := true, false

	switch flag {
	case "yes":
		return &yes, nil
	case "no":
		return &no, nil
	}

	// Already answered: don't prompt again.
	if cfg.AutoSync != nil {
		return nil, nil
	}

	// First run: prompt only on a real terminal.
	if file, ok := in.(*os.File); ok && term.IsTerminal(file.Fd()) {
		answered, err := promptAutoSync(in, out)
		if err != nil {
			return nil, err
		}
		if answered {
			return &yes, nil
		}
		return &no, nil
	}

	return nil, nil
}

func promptAutoSync(in io.Reader, out io.Writer) (bool, error) {
	fmt.Fprint(out, "Sync automatically when specs change? [y/N] ")
	br := bufio.NewReader(in)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	return strings.ToLower(strings.TrimSpace(line)) == "y", nil
}

// writeAutoSyncSpec writes the auto-sync rule spec to rulesDir.
// Skips when the file already exists (idempotent).
func writeAutoSyncSpec(rulesDir string) error {
	path := filepath.Join(rulesDir, autoSyncSpecName+".md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(autoSyncSpecContent), 0o644)
}

func validateAutoSyncFlag(flag string) error {
	switch flag {
	case "", "yes", "no":
		return nil
	}
	return fmt.Errorf("--auto-sync: expected 'yes' or 'no', got %q", flag)
}
