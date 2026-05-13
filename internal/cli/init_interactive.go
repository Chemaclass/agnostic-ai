package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

// This file holds the canonical target list and the selection helpers
// shared by the default and interactive init flows. selectTargets
// branches on TTY vs piped input.

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
	{Name: "windsurf", Desc: "Windsurf (Codeium)"},
	{Name: "continue", Desc: "Continue (continue.dev)"},
	{Name: "amp", Desc: "Amp (Sourcegraph)"},
	{Name: "zed", Desc: "Zed editor"},
	{Name: "warp", Desc: "Warp terminal"},
	{Name: "opencode", Desc: "OpenCode"},
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

// selectTargets returns the user's chosen subset of allTargets, by
// canonical name. Branches on whether `in` is a terminal-backed
// *os.File (run an interactive multi-select via huh) or a plain reader
// (read one line and parsePipedSelection it). On immediate EOF with
// no terminal, returns a clear error.
func selectTargets(in io.Reader, stderr io.Writer) ([]string, error) {
	if f, ok := in.(*os.File); ok && term.IsTerminal(f.Fd()) {
		return runInteractivePrompt(stderr)
	}

	br := bufio.NewReader(in)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	if line == "" {
		return nil, fmt.Errorf("init -i requires an interactive terminal or piped target list")
	}
	return parsePipedSelection(line)
}

// runInteractivePrompt drives the huh multi-select. It is glue around
// the third-party widget; behavior is verified manually rather than
// in unit tests.
func runInteractivePrompt(stderr io.Writer) ([]string, error) {
	opts := make([]huh.Option[string], len(allTargets))
	for i, t := range allTargets {
		label := fmt.Sprintf("%-9s %s", t.Name, t.Desc)
		opts[i] = huh.NewOption(label, t.Name)
	}
	var picked []string
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
