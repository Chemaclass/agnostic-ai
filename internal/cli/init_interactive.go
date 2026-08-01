package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

// This file holds the canonical target list and the selection helpers
// shared by the default init flow and the first-sync target picker.
// The TTY-aware multi-mode entry point is selectTargetsForSync in
// sync_picker.go.

// targetChoice is one selectable AI CLI target shown to the user and
// written to agnostic-ai.yaml.
type targetChoice struct {
	Name string // canonical name written to the config
	Desc string // short human description shown in prompts
}

// allTargets is the canonical target list, in display + emit order.
// Single source of truth for renderConfig and the interactive prompt.
var allTargets = []targetChoice{
	{Name: "claude", Desc: "Claude Code (Anthropic)"},
	{Name: "codex", Desc: "Codex CLI (OpenAI)"},
	{Name: "gemini", Desc: "Gemini CLI (Google)"},
	{Name: "cursor", Desc: "Cursor (cursor.com)"},
	{Name: "copilot", Desc: "GitHub Copilot"},
	{Name: "aider", Desc: "Aider (aider.chat)"},
	{Name: "cline", Desc: "Cline (VSCode extension)"},
	{Name: "windsurf", Desc: "Windsurf / Devin Desktop"},
	{Name: "continue", Desc: "Continue (continue.dev)"},
	{Name: "amp", Desc: "Amp (Sourcegraph)"},
	{Name: "zed", Desc: "Zed editor"},
	{Name: "warp", Desc: "Warp terminal"},
	{Name: "opencode", Desc: "OpenCode"},
	{Name: "antigravity", Desc: "Google Antigravity"},
	{Name: "junie", Desc: "Junie (JetBrains)"},
	{Name: "kiro", Desc: "Kiro (AWS)"},
	{Name: "crush", Desc: "Crush (Charm)"},
	{Name: "trae", Desc: "Trae (ByteDance)"},
	{Name: "qoder", Desc: "Qoder (Alibaba)"},
	{Name: "openhands", Desc: "OpenHands (All Hands)"},
	{Name: "factory", Desc: "Factory Droid"},
	{Name: "kilo", Desc: "Kilo Code"},
	{Name: "jules", Desc: "Jules (Google, cloud)"},
	{Name: "goose", Desc: "Goose (Block)"},
	{Name: "augment", Desc: "Augment Code"},
}

// allTargetNames returns just the canonical names of allTargets.
func allTargetNames() []string {
	out := make([]string, len(allTargets))
	for i, t := range allTargets {
		out[i] = t.Name
	}
	return out
}

// errNoTargets is returned when a selection (interactive or piped)
// resolves to zero targets. Surfaced to the user with a clear message.
var errNoTargets = errors.New("no targets selected; rerun and pick at least one")

// parsePipedSelection takes a single line of comma-separated target
// names, trims whitespace around each entry, deduplicates while
// preserving canonical (allTargets) order, and validates every name
// against allTargets. An empty or whitespace-only line returns
// errNoTargets. An unknown name returns a descriptive error listing
// the valid names.
func parsePipedSelection(line string) ([]string, error) {
	raw := strings.Split(strings.TrimSpace(line), ",")
	picked := make(map[string]bool, len(raw))
	for _, r := range raw {
		name := strings.TrimSpace(r)
		if name == "" {
			continue
		}
		if !isKnownTarget(name) {
			return nil, fmt.Errorf("unknown target %q (valid: %s)",
				name, strings.Join(allTargetNames(), ", "))
		}
		picked[name] = true
	}
	if len(picked) == 0 {
		return nil, errNoTargets
	}
	return filterToCanonicalOrder(picked), nil
}

// isKnownTarget reports whether name matches any entry in allTargets.
func isKnownTarget(name string) bool {
	for _, t := range allTargets {
		if t.Name == name {
			return true
		}
	}
	return false
}

// filterToCanonicalOrder returns the subset of allTargets whose names
// appear in picked, preserving allTargets order.
func filterToCanonicalOrder(picked map[string]bool) []string {
	out := make([]string, 0, len(picked))
	for _, t := range allTargets {
		if picked[t.Name] {
			out = append(out, t.Name)
		}
	}
	return out
}

// runInteractivePrompt drives the huh multi-select. It is glue around
// the third-party widget; behavior is verified manually rather than
// in unit tests. preselected names are pre-ticked on entry.
func runInteractivePrompt(stderr io.Writer, preselected []string) ([]string, error) {
	opts := make([]huh.Option[string], len(allTargets))
	for i, t := range allTargets {
		label := fmt.Sprintf("%-9s %s", t.Name, t.Desc)
		opts[i] = huh.NewOption(label, t.Name)
	}
	picked := make([]string, 0, len(preselected))
	known := map[string]bool{}
	for _, n := range preselected {
		if !known[n] && isKnownTarget(n) {
			known[n] = true
			picked = append(picked, n)
		}
	}
	form := huh.NewMultiSelect[string]().
		Title("Select targets to enable").
		Options(opts...).
		Value(&picked)
	if err := form.Run(); err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, errNoTargets
	}
	pickedSet := make(map[string]bool, len(picked))
	for _, n := range picked {
		pickedSet[n] = true
	}
	return filterToCanonicalOrder(pickedSet), nil
}

// promptGitignoreEnable asks the user whether to enable
// gitignore.enabled in the rendered config. Only TTY stdin runs the
// confirm widget; piped or closed stdin falls back to true so first-time
// and CI inits ignore generated outputs without a prompt (pass
// --gitignore=false to commit them instead).
//
// Defaults to true: the source specs under .agnostic-ai/ are the one
// committed copy, and treating the emitted target files (CLAUDE.md,
// AGENTS.md, .cursor/, ...) as build artifacts keeps them out of git
// and review noise. Flip off if teammates lack the CLI and need the
// generated conventions committed.
func promptGitignoreEnable(in io.Reader) (bool, error) {
	f, ok := in.(*os.File)
	if !ok || !term.IsTerminal(f.Fd()) {
		return true, nil
	}
	picked := true
	form := huh.NewConfirm().
		Title("Ignore generated target files in .gitignore?").
		Description("`sync` will keep a managed block of every emitted target path. On by default; flip off if your team commits emitted files so teammates without the CLI still see the conventions.").
		Affirmative("Yes, ignore them").
		Negative("No, commit them").
		Value(&picked)
	if err := form.Run(); err != nil {
		return false, err
	}
	return picked, nil
}

// targetMarkers maps each canonical target to filesystem paths that
// indicate the project already uses that CLI. A target is "detected"
// when at least one of its markers exists. Markers are chosen to be
// exclusive (no shared root files like AGENTS.md) so detection does not
// over-tick.
var targetMarkers = map[string][]string{
	"claude":      {".claude"},
	"codex":       {".codex", ".agents/agents"},
	"gemini":      {".gemini"},
	"cursor":      {".cursor"},
	"copilot":     {".github/copilot-instructions.md", ".github/instructions"},
	"aider":       {".aider.conf.yml", ".aider.conf.yaml"},
	"cline":       {".cline", ".clinerules"},
	"windsurf":    {".devin", ".windsurf"},
	"continue":    {".continue"},
	"amp":         {".amp"},
	"zed":         {".zed"},
	"warp":        {".warp"},
	"opencode":    {".opencode"},
	"antigravity": {".agent", ".agents/rules"},
	"junie":       {".junie"},
	"kiro":        {".kiro"},
	"crush":       {"crush.json", ".crush"},
	"trae":        {".trae"},
	"qoder":       {".qoder"},
	"openhands":   {".openhands"},
	"factory":     {".factory"},
	"kilo":        {".kilo", "kilo.jsonc", ".kilocode"},
	"goose":       {".goosehints"},
	"augment":     {".augment", ".augment-guidelines"},
}

// detectExistingTargets returns the canonical-ordered subset of
// allTargets whose marker paths exist under root. Used by `init` to
// pre-tick CLIs the user already has configured.
func detectExistingTargets(root string) []string {
	picked := map[string]bool{}
	for _, t := range allTargets {
		for _, marker := range targetMarkers[t.Name] {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				picked[t.Name] = true
				break
			}
		}
	}
	return filterToCanonicalOrder(picked)
}
