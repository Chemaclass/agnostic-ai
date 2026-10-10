package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
// A target whose `outputs.<target>.rules-file` names its own
// entry-point file is skipped: the adapter owns that write. A
// rules-file pointing anywhere else is not a collision, so the target
// keeps its entry-point file.
//
// A hand-authored entry-point file (no agnostic-ai provenance marker)
// triggers a one-line warning before the overwrite so the user knows
// their content is about to be replaced. This is the same heuristic
// the importer uses to skip files it did not write.
func writeAgnosticEntryPoints(sess *adapters.Session, cfg *config.Config, b spec.Bundle, targets []string, dryRun bool) error {
	body, err := resolveAgnosticBody(sess, dryRun)
	if err != nil {
		return err
	}
	files, err := renderEntryPointFiles(cfg, b, targets, body)
	if err != nil {
		return err
	}
	for _, f := range files {
		// A user-owned file is expected to be hand-authored. sess still
		// receives the write so it records the skip for the summary.
		if !cfg.IsUnmanaged(f.Path) {
			warnOnHandAuthoredEntryPoint(f.Path, body)
		}
		if err := sess.WriteFile(f.Path, f.Content, dryRun); err != nil {
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
	// Readers are the targets that load Path.
	Readers []string
	// Layers is the text each source adds to Content, for lint's
	// instructions budget.
	Layers []instructionLayer
	// plain is set when Content is the instructions body and the local
	// layer alone, with no rules, review, or overview block.
	plain bool
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
//
// Targets with no native rules directory (codex, amp, warp, gemini,
// aider, opencode) get the rule bodies inlined into their entry-point
// file via a second sentinel-marked block, since that file is their
// only always-on context surface. The block is identical across the
// consumers of a shared path (rules are global), so the deduplicated
// write stays collision-free.
//
// The body may carry ::target fences (see spec.FilterFences). A fenced
// block reaches a path when any of that path's consumers is listed, so a
// shared AGENTS.md keeps a codex-only block for every AGENTS.md reader; a
// shared file is never split. AGNOSTIC_AI.md itself keeps the fences.
//
// The git-ignored `.agnostic-ai/local/AGNOSTIC_AI.md`, when present,
// extends the shared body in a sentinel-marked block after the rules
// block, so personal text has the last word and import can drop it.
// Fences and imports resolve in it exactly as in the shared body.
func renderEntryPointFiles(cfg *config.Config, b spec.Bundle, targets []string, body string) ([]entryPointFile, error) {
	cfg = cfg.WithAdditionalTargets(targets...)
	body = adapters.StripGeneratedAppendices(body)

	entryPoint := func(t string) string {
		if adapters.LegacyRulesFileOwnsEntryPoint(cfg, t) {
			return ""
		}
		if path := adapters.EntryPointPath(cfg, t); path != adapters.AgnosticEntryPointPath {
			return path
		}
		return ""
	}
	var order []string
	consumers := map[string][]string{}
	for _, t := range targets {
		if path := entryPoint(t); path != "" {
			if _, ok := consumers[path]; !ok {
				order = append(order, path)
				consumers[path] = nil
			}
		}
	}
	// A partial run writes the same file a full one does: every
	// configured reader counts, so the rules block its own rule files
	// leave to that file stays in it.
	seen := map[string]bool{}
	for _, t := range append(slices.Clone(cfg.Targets), targets...) {
		path := entryPoint(t)
		if _, ok := consumers[path]; !ok || seen[t] {
			continue
		}
		seen[t] = true
		consumers[path] = append(consumers[path], t)
	}

	local, err := adapters.ReadLocalInstructions()
	if err != nil {
		return nil, err
	}

	files := make([]entryPointFile, 0, len(order))
	for _, path := range order {
		content, imports, err := entryPointViewWithImports(cfg, path, consumers[path], body)
		if err != nil {
			return nil, err
		}
		layers := []instructionLayer{{Name: "AGNOSTIC_AI.md", Text: content, Sources: []instructionSource{{Path: adapters.AgnosticEntryPointPath, Text: content, Imports: imports}}}}
		plain := !cfg.Sync.TargetOverview
		unchanged := content
		if inliners := pathRuleInliners(cfg, consumers[path]); len(inliners) > 0 {
			var rulesAppendix string
			for i, target := range inliners {
				next := adapters.RenderRulesAppendix(adapters.EntryPointRules(b, target, cfg))
				if i > 0 && next != rulesAppendix {
					return nil, fmt.Errorf("%s: target-specific root rules differ between readers; use compatible target conditions or separate worktrees", path)
				}
				rulesAppendix = next
			}
			adapters.NoteEntryPointVars(cfg, b, inliners[0])
			content = adapters.AppendRulesAppendix(content, rulesAppendix)
			layers = append(layers, instructionLayer{Name: "rules", Text: rulesAppendix, Sources: ruleInstructionSources(adapters.EntryPointRules(b, inliners[0], cfg))})
		} else if importer := pathRulesImporter(cfg, consumers[path]); importer != "" {
			content = adapters.AppendRulesAppendix(content, adapters.RenderRulesImportAppendix(cfg, importer, adapters.EntryPointRules(b, importer, cfg)))
		} else if importer := pathLegacyRulesFileImporter(cfg, consumers[path]); importer != "" {
			content = adapters.AppendRulesAppendix(content, adapters.RenderLegacyRulesFileImportAppendix(cfg, importer))
		}
		plain = plain && content == unchanged
		if slices.Contains(consumers[path], "codex") {
			if section := adapters.ReviewSections(b, cfg, targets...)[""]; section != "" {
				plain = false
				content = adapters.AppendReviewSection(content, section)
				layers = append(layers, instructionLayer{Name: "reviews", Text: section, Sources: reviewInstructionSources(cfg, b, targets...)})
			}
		}
		if local != "" {
			localView, imports, err := entryPointViewWithImports(cfg, path, consumers[path], local)
			if err != nil {
				return nil, err
			}
			content = adapters.AppendLocalInstructions(content, localView)
			layers = append(layers, instructionLayer{Name: "local/AGNOSTIC_AI.md", Text: localView, Sources: []instructionSource{{Path: adapters.ProjectLocalEntryPointPath, Text: localView, Imports: imports}}})
		}
		memory, err := memoryBlockFor(cfg, path, consumers[path])
		if err != nil {
			return nil, err
		}
		if memory != "" {
			content = adapters.AppendMemoryBlock(content, memory)
			layers = append(layers, instructionLayer{Name: "shared memory", Text: memory})
		}
		if cfg.Sync.TargetOverview {
			var sections []adapters.TargetArtifacts
			for _, t := range consumers[path] {
				sections = append(sections, adapters.TargetArtifacts{
					Target:    t,
					Artifacts: adapters.NativeArtifactsFor(t, cfg),
				})
			}
			content = adapters.AppendTargetOverview(content, adapters.RenderTargetOverview(sections))
		}
		// Mirror sess.WriteFile's trailing-newline normalization so Content
		// equals the bytes on disk and the drift check never false-positives
		// (an AGNOSTIC_AI.md ending in several newlines, fenced or not).
		rendered := renderContextInstructions(content, layers)
		if rendered != "" {
			rendered = strings.TrimRight(rendered, "\n") + "\n"
		}
		files = append(files, entryPointFile{
			Path:    path,
			Content: rendered,
			Readers: consumers[path],
			Layers:  layers,
			plain:   plain,
		})
	}
	return importAgentsFromClaude(cfg, files, body, local)
}

// importAgentsFromClaude rewrites the root CLAUDE.md as `@AGENTS.md` plus
// the text only Claude Code reads, when another target writes the root
// AGENTS.md. Cursor loads both files, so two full copies cost every Cursor
// session the instructions twice, and Claude Code expands the import to
// the same text. It applies only when both files hold the instructions
// alone and Claude Code would see exactly AGENTS.md's text around its own
// `::target claude` blocks; otherwise both keep the full text.
func importAgentsFromClaude(cfg *config.Config, files []entryPointFile, body, local string) ([]entryPointFile, error) {
	claudeAt, agentsAt := -1, -1
	for i, f := range files {
		switch {
		case f.Path == "CLAUDE.md" && slices.Equal(f.Readers, []string{"claude"}):
			claudeAt = i
		case f.Path == "AGENTS.md" && !slices.Contains(f.Readers, "claude"):
			agentsAt = i
		}
	}
	if claudeAt >= 0 && agentsAt < 0 && files[claudeAt].plain && claudeImportsAgentsOnDisk() {
		// The project keeps its instructions in AGENTS.md, CLAUDE.md
		// imports it, and no other target writes AGENTS.md: Claude Code's
		// sync writes it, so an edit to AGENTS.md keeps reaching Claude.
		return claudeWritesAgents(cfg, files, claudeAt, body, local)
	}
	if claudeAt < 0 || agentsAt < 0 || !files[claudeAt].plain || !files[agentsAt].plain {
		return files, nil
	}
	agents := files[agentsAt]
	parts := []string{"@AGENTS.md"}
	var companionLayers []instructionLayer
	for i, text := range []string{body, local} {
		if text == "" {
			continue
		}
		rest, only := spec.SplitReaderOnly(text, "claude", agents.Readers)
		claudeRest, err := entryPointView(cfg, files[claudeAt].Path, []string{"claude"}, rest)
		if err != nil {
			return nil, err
		}
		agentsView, err := entryPointView(cfg, agents.Path, agents.Readers, text)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(claudeRest) != strings.TrimSpace(agentsView) {
			return files, nil
		}
		if only != "" {
			parts = append(parts, only)
			name, source := "AGNOSTIC_AI.md", adapters.AgnosticEntryPointPath
			if i == 1 {
				name, source = "local/AGNOSTIC_AI.md", adapters.ProjectLocalEntryPointPath
			}
			companionLayers = append(companionLayers, instructionLayer{Name: name + " (Claude Code only)", Text: only, Sources: []instructionSource{{Path: source, Text: only}}})
		}
	}
	parts, err := appendMemoryPart(cfg, parts, files[claudeAt].Path)
	if err != nil {
		return nil, err
	}
	companion := renderContextInstructions(strings.Join(parts, "\n\n")+"\n", companionLayers)
	files[claudeAt].Content = strings.TrimRight(companion, "\n") + "\n"
	files[claudeAt].Layers = companionLayers
	return files, nil
}

// claudeWritesAgents writes the shared body to AGENTS.md and makes
// CLAUDE.md `@AGENTS.md` plus the ::target claude blocks, the layout the
// project already has, instead of one CLAUDE.md with the whole text.
func claudeWritesAgents(cfg *config.Config, files []entryPointFile, claudeAt int, body, local string) ([]entryPointFile, error) {
	parts := []string{"@AGENTS.md"}
	var shared []string
	var sharedSources []instructionSource
	var companionLayers []instructionLayer
	for i, text := range []string{body, local} {
		if text == "" {
			continue
		}
		rest, only := spec.SplitReaderOnly(text, "claude", nil)
		view, imports, err := entryPointViewWithImports(cfg, "AGENTS.md", nil, rest)
		if err != nil {
			return nil, err
		}
		shared = append(shared, view)
		name, source := "AGNOSTIC_AI.md", adapters.AgnosticEntryPointPath
		if i == 1 {
			name, source = "local/AGNOSTIC_AI.md", adapters.ProjectLocalEntryPointPath
		}
		sharedSources = append(sharedSources, instructionSource{Path: source, Text: view, Imports: imports})
		if only != "" {
			parts = append(parts, only)
			companionLayers = append(companionLayers, instructionLayer{Name: name + " (Claude Code only)", Text: only, Sources: []instructionSource{{Path: source, Text: only}}})
		}
	}
	if len(shared) == 0 {
		return files, nil
	}
	agentsText := shared[0]
	if len(shared) > 1 {
		agentsText = adapters.AppendLocalInstructions(agentsText, shared[1])
	}
	sharedLayers := []instructionLayer{{Name: "AGNOSTIC_AI.md", Text: agentsText, Sources: sharedSources}}
	rendered := strings.TrimRight(renderContextInstructions(agentsText, sharedLayers), "\n") + "\n"
	parts, err := appendMemoryPart(cfg, parts, files[claudeAt].Path)
	if err != nil {
		return nil, err
	}
	companion := renderContextInstructions(strings.Join(parts, "\n\n")+"\n", companionLayers)
	files[claudeAt].Content = strings.TrimRight(companion, "\n") + "\n"
	files[claudeAt].Layers = companionLayers
	return append(files, entryPointFile{
		Path:    "AGENTS.md",
		Content: rendered,
		Readers: []string{"claude"},
		Layers:  sharedLayers,
		plain:   true,
	}), nil
}

// Reuse the renderer's source entries and section text rather than treating
// marker-looking comments inside authored bodies as section boundaries.
func ruleInstructionSources(b spec.Bundle) []instructionSource {
	var sources []instructionSource
	for _, r := range b.Rules {
		section := adapters.RenderRulesAppendix(spec.Bundle{Rules: []spec.Entry{r}})
		if section == "" {
			continue
		}
		section = strings.TrimPrefix(section, "<!-- agnostic-ai:rules:start -->\n\n## Rules\n\n")
		section = strings.TrimSuffix(section, "<!-- agnostic-ai:rules:end -->\n")
		sources = append(sources, instructionSource{Path: adapters.EntrySourcePath(r), Text: section})
	}
	return sources
}

func reviewInstructionSources(cfg *config.Config, b spec.Bundle, targets ...string) []instructionSource {
	var sources []instructionSource
	for _, r := range b.For("codex").Reviews {
		section := adapters.ReviewSections(spec.Bundle{Reviews: []spec.Entry{r}}, cfg, targets...)[""]
		if section == "" {
			continue
		}
		section = strings.TrimPrefix(section, "<!-- agnostic-ai:reviews:start -->\n\n## Code Review Rules\n\n")
		section = strings.TrimSuffix(section, "\n\n<!-- agnostic-ai:reviews:end -->\n")
		sources = append(sources, instructionSource{Path: adapters.EntrySourcePath(r), Text: section})
	}
	return sources
}

// claudeImportsAgentsOnDisk reports whether the root CLAUDE.md is the
// `@AGENTS.md` companion, hand-written or synced, with AGENTS.md next to it.
func claudeImportsAgentsOnDisk() bool {
	raw, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		return false
	}
	if _, err := os.Stat("AGENTS.md"); err != nil {
		return false
	}
	_, ok := adapters.SplitAgentsCompanion(header.Strip(adapters.StripGeneratedAppendices(string(raw))))
	return ok
}

// entryPointView returns text as the readers of path see it: ::target
// fences resolved for those readers, and `@path` imports rewritten per
// sync.resolve-imports when a reader cannot follow them.
func entryPointView(cfg *config.Config, path string, readers []string, text string) (string, error) {
	view, _, err := entryPointViewWithImports(cfg, path, readers, text)
	return view, err
}

func entryPointViewWithImports(cfg *config.Config, path string, readers []string, text string) (string, []adapters.ImportSpan, error) {
	view := spec.FilterFences(text, readers)
	if pathSupportsFileImports(readers) {
		return view, nil, nil
	}
	resolved, spans, err := adapters.ApplyImportModeWithSpans(view, cfg.Sync.ResolveImports)
	if err != nil {
		return "", nil, fmt.Errorf("resolve imports for %s: %w", path, err)
	}
	return resolved, spans, nil
}

// pathRuleInliners returns the targets consuming an entry-point path
// that inline rule bodies into it. A shared path (AGENTS.md) inlines
// when at least one consumer needs it; the block is identical for all.
//
// A consumer on the legacy concatenated rules-file layout is excluded:
// its adapter already owns rule delivery, so inlining the same bodies
// into the entry point would ship every rule twice. Mirrors the same
// predicate in render.go's entryPointRuleFile so `render` and `sync`
// never disagree on what the file holds.
func pathRuleInliners(cfg *config.Config, consumers []string) []string {
	var out []string
	for _, t := range consumers {
		if adapters.InlinesRulesIntoEntryPoint(t) && !adapters.HasLegacyRulesFile(cfg, t) {
			out = append(out, t)
		}
	}
	return out
}

// pathLegacyRulesFileImporter returns the first consumer of an
// entry-point path whose legacy concatenated rules-file sits off that
// path and whose CLI resolves `@`-imports (claude), or "" when none
// does. Without the import line the concatenated file reaches no
// session: `.claude/RULES.md` is on no documented Claude Code load path.
func pathLegacyRulesFileImporter(cfg *config.Config, consumers []string) string {
	for _, t := range consumers {
		if adapters.RenderLegacyRulesFileImportAppendix(cfg, t) != "" {
			return t
		}
	}
	return ""
}

// pathSupportsFileImports reports whether every target consuming an
// entry-point path resolves `@path` file-imports natively. When false,
// sync applies the resolve-imports mode so non-resolving targets do not
// carry dead reference lines. Claude's CLAUDE.md has a single resolving
// consumer; shared paths (AGENTS.md) have none.
func pathSupportsFileImports(consumers []string) bool {
	for _, t := range consumers {
		if !adapters.SupportsFileImports(t) {
			return false
		}
	}
	return true
}

// pathRulesImporter returns the first consumer of an entry-point path
// that opted into `@`-import rule wiring (outputs.<target>.rules-mode:
// import), or "" when none did. Claude's CLAUDE.md has a single consumer,
// so the first match is the only one.
func pathRulesImporter(cfg *config.Config, consumers []string) string {
	for _, t := range consumers {
		if adapters.ImportsRulesIntoEntryPoint(cfg, t) {
			return t
		}
	}
	return ""
}

// entryPointPaths returns the entry-point files sync distributes: the
// canonical AGNOSTIC_AI.md plus each enabled target's native file
// (CLAUDE.md, AGENTS.md, GEMINI.md, ...), deduplicated and in target order.
// A target whose legacy concatenated rules-file names its own
// entry-point file is skipped because the adapter owns that write.
// Mirrors writeAgnosticEntryPoints' path selection so revert and check
// stay symmetric with sync (#389).
// User-owned paths (sync.unmanaged) are excluded so revert and cleanup
// never touch them.
func entryPointPaths(cfg *config.Config, targets []string) []string {
	seen := map[string]bool{adapters.AgnosticEntryPointPath: true}
	paths := []string{adapters.AgnosticEntryPointPath}
	for _, t := range targets {
		if adapters.LegacyRulesFileOwnsEntryPoint(cfg, t) {
			continue
		}
		path := adapters.EntryPointPath(cfg, t)
		if path == "" || seen[path] || cfg.IsUnmanaged(path) {
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
func warnOnHandAuthoredEntryPoint(path, captured string) {
	// Text AGNOSTIC_AI.md or the local layer holds is not lost, so there
	// is nothing to warn about.
	held, err := heldInstructions(captured)
	if err != nil {
		return
	}
	if uncaptured, err := handWrittenUncaptured(path, held); err != nil || !uncaptured {
		return
	}
	summaryf("  ! %s appears hand-authored (no agnostic-ai header) — overwriting with the canonical pointer body. Move custom content into %s first to keep it.\n",
		path, adapters.AgnosticEntryPointPath)
}

// resolveAgnosticBody returns the raw (no header) body for entry-point files.
// When AGNOSTIC_AI.md exists its content drives all targets and is never
// rewritten; when absent the template is written to AGNOSTIC_AI.md without
// the provenance header, since it is the file the user edits.
func resolveAgnosticBody(sess *adapters.Session, dryRun bool) (string, error) {
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err == nil {
		return header.Strip(string(data)), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", adapters.AgnosticEntryPointPath, err)
	}
	body := adapters.EntryPointBody()
	if err := sess.WriteFile(adapters.AgnosticEntryPointPath, body, dryRun); err != nil {
		return "", fmt.Errorf("write %s: %w", adapters.AgnosticEntryPointPath, err)
	}
	return body, nil
}

// memoryBuiltin names the built-in whose shared memory index sync
// imports into entry points.
const memoryBuiltin = "memory"

// memoryBlockFor returns the shared memory import block for the
// entry-point file at path, or "" when the memory built-in is off or a
// reader of path cannot follow `@` lines. Those readers find the index
// through the always-on shared-memory rule instead, so the block never
// reaches a file resolve-imports rewrites.
func memoryBlockFor(cfg *config.Config, path string, readers []string) (string, error) {
	if !slices.Contains(cfg.Builtins, memoryBuiltin) || len(readers) == 0 || !pathSupportsFileImports(readers) {
		return "", nil
	}
	dir, err := adapters.PersonalMemoryDirFor(cfg, path, readers...)
	if err != nil {
		return "", err
	}
	return adapters.RenderMemoryBlock(path, filepath.Join(dir, "MEMORY.md")), nil
}

// appendMemoryPart adds the shared memory import block to the parts of
// the CLAUDE.md companion at path.
func appendMemoryPart(cfg *config.Config, parts []string, path string) ([]string, error) {
	memory, err := memoryBlockFor(cfg, path, []string{"claude"})
	if err != nil || memory == "" {
		return parts, err
	}
	return append(parts, strings.TrimRight(memory, "\n")), nil
}

func renderContextInstructions(text string, layers []instructionLayer) string {
	rendered, copied := header.WithCopiedRanges(text, header.FormatMarkdown)
	if len(layers) > 0 {
		layers[0].Rendering = &instructionRendering{Text: text, Copied: copied}
	}
	return rendered
}
