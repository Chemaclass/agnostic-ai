package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// writeAgnosticEntryPoints distributes the canonical entry-point body to
// every enabled target's native file (CLAUDE.md, AGENTS.md, GEMINI.md, ...).
//
// The body source is .agnostic-ai/AGNOSTIC_AI.md:
//   - If it exists, its content (header stripped) is used as-is. This lets
//     `import <target>` seed the file and have sync propagate that content.
//   - If it is absent, the generated template body is written to
//     AGNOSTIC_AI.md first, then distributed to targets.
//
// Targets that opted into the legacy concatenated rules-file layout
// via `outputs.<target>.rules-file` are skipped: the adapter owns the
// entry-point write in that case.
//
// A hand-authored entry-point file (no agnostic-ai provenance marker)
// triggers a one-line warning before the overwrite so the user knows
// their content is about to be replaced. This is the same heuristic
// the importer uses to skip files it did not write.
func writeAgnosticEntryPoints(cfg *config.Config, targets []string, dryRun bool) error {
	body, err := resolveAgnosticBody(cfg, dryRun)
	if err != nil {
		return err
	}
	for _, f := range renderEntryPointFiles(cfg, targets, body) {
		warnOnHandAuthoredEntryPoint(f.Path)
		if err := adapters.WriteFile(f.Path, f.Content, dryRun); err != nil {
			return fmt.Errorf("write entry-point %s: %w", f.Path, err)
		}
	}
	return nil
}

// entryPointFile pairs an entry-point path with the exact content sync
// writes there. Shared by writeAgnosticEntryPoints and the drift check
// so the two can never disagree on what a target's file should hold.
type entryPointFile struct {
	Path    string
	Content string
}

// renderEntryPointFiles returns every target entry-point file sync
// distributes for body, deduplicated by path in target order.
// AGNOSTIC_AI.md is excluded: it holds the canonical body and is
// written by resolveAgnosticBody, never with an overview appendix.
//
// When sync.target-overview is enabled, each file gains a generated
// sentinel-marked appendix listing the native artifact locations of
// every target consuming that path (a shared AGENTS.md lists each
// consumer separately).
func renderEntryPointFiles(cfg *config.Config, targets []string, body string) []entryPointFile {
	body = adapters.StripTargetOverview(body)
	var order []string
	consumers := map[string][]string{}
	for _, t := range targets {
		if adapters.HasLegacyRulesFile(cfg, t) {
			continue
		}
		path := adapters.EntryPointPath(cfg, t)
		if path == "" || path == adapters.AgnosticEntryPointPath {
			continue
		}
		if _, ok := consumers[path]; !ok {
			order = append(order, path)
		}
		consumers[path] = append(consumers[path], t)
	}

	files := make([]entryPointFile, 0, len(order))
	for _, path := range order {
		content := body
		if cfg.Sync.TargetOverview {
			var sections []adapters.TargetArtifacts
			for _, t := range consumers[path] {
				sections = append(sections, adapters.TargetArtifacts{
					Target:    t,
					Artifacts: adapters.NativeArtifactsFor(t, cfg),
				})
			}
			content = adapters.AppendTargetOverview(body, adapters.RenderTargetOverview(sections))
		}
		files = append(files, entryPointFile{
			Path:    path,
			Content: header.With(content, header.FormatMarkdown),
		})
	}
	return files
}

// entryPointPaths returns the entry-point files sync distributes: the
// canonical AGNOSTIC_AI.md plus each enabled target's native file
// (CLAUDE.md, AGENTS.md, GEMINI.md, ...), deduplicated and in target order.
// Targets on the legacy concatenated rules-file layout are skipped because
// the adapter owns that write. Mirrors writeAgnosticEntryPoints' path
// selection so revert and check stay symmetric with sync (#389).
func entryPointPaths(cfg *config.Config, targets []string) []string {
	seen := map[string]bool{adapters.AgnosticEntryPointPath: true}
	paths := []string{adapters.AgnosticEntryPointPath}
	for _, t := range targets {
		if adapters.HasLegacyRulesFile(cfg, t) {
			continue
		}
		path := adapters.EntryPointPath(cfg, t)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}
	return paths
}

// warnOnHandAuthoredEntryPoint prints a single-line warning when path
// exists, has non-empty content, and lacks the agnostic-ai provenance
// marker. The marker is the same one `header.Has` uses to recognize a
// generated file. Read errors are swallowed: sync proceeds with the
// overwrite either way (preserving the historic behavior) but a quiet
// I/O failure does not block the user.
func warnOnHandAuthoredEntryPoint(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return
	}
	if header.Has(string(data)) {
		return
	}
	summaryf("  ! %s appears hand-authored (no agnostic-ai header) — overwriting with the canonical pointer body. Move custom content into %s first to keep it.\n",
		path, adapters.AgnosticEntryPointPath)
}

// resolveAgnosticBody returns the raw (no header) body for entry-point files.
// When AGNOSTIC_AI.md exists its content drives all targets; when absent the
// template is generated, written to AGNOSTIC_AI.md, and returned.
func resolveAgnosticBody(cfg *config.Config, dryRun bool) (string, error) {
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err == nil {
		return header.Strip(string(data)), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}
	body := adapters.EntryPointBody(cfg)
	rendered := header.With(body, header.FormatMarkdown)
	if err := adapters.WriteFile(adapters.AgnosticEntryPointPath, rendered, dryRun); err != nil {
		return "", fmt.Errorf("write %s: %w", adapters.AgnosticEntryPointPath, err)
	}
	return body, nil
}
