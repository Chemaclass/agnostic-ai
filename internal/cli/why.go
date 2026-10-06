package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// whySource describes one source spec that contributes to the emitted file.
type whySource struct {
	Kind    string      `json:"kind"`
	Name    string      `json:"name"`
	Path    string      `json:"path"`
	Mode    string      `json:"mode"` // "full" or "section"
	Layer   string      `json:"layer,omitempty"`
	Builtin *builtinRef `json:"builtin,omitempty"`
}

// whyOutput is the JSON envelope for `why --format json`.
type whyOutput struct {
	Version    string      `json:"version"`
	Command    string      `json:"command"`
	File       string      `json:"file"`
	Target     string      `json:"target"`
	Configured bool        `json:"configured"`
	OutputKeys []string    `json:"output_keys"`
	Sources    []whySource `json:"sources"`
	LastSync   *string     `json:"last_sync"`
}

func newWhyCmd() *cobra.Command {
	var format string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "why <file>",
		Short: "Trace an emitted file back to the source spec(s) and adapter that produced it.",
		Long: "Reverse provenance for an emitted file. Reports the adapter that " +
			"wrote it, the source spec(s) it came from, the `outputs.<target>.*` " +
			"config keys used to resolve the path, and the last sync timestamp.",
		Example: `  # Human-readable
  agnostic-ai why .claude/rules/no-console-log.md

  # Machine-readable
  agnostic-ai why .claude/rules/no-console-log.md --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if format != "" && format != "text" && format != "json" {
				return fmt.Errorf("--format: expected 'text' or 'json', got %q", format)
			}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			report, err := traceFile(args[0], cfg, bundle, ".")
			if err != nil {
				return err
			}
			if format == "json" || jsonOut {
				return emitWhyJSON(cmd, report)
			}
			printWhyText(cmd, report)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json.")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

// traceFile resolves the user-supplied path against every adapter in the
// registry. Returns the matching target, the source specs that contribute
// to the file, the outputs.<target>.* keys used to derive the path, and
// the last sync timestamp.
func traceFile(input string, cfg *config.Config, b spec.Bundle, projectRoot string) (whyOutput, error) {
	resolved, rel, err := normalizeInputPath(input, projectRoot)
	if err != nil {
		return whyOutput{}, err
	}
	if filepath.ToSlash(rel) == filepath.ToSlash(adapters.AgnosticEntryPointPath) {
		return whyOutput{}, fmt.Errorf("%s is the shared instructions body, not a generated file. sync copies it into every target's entry point (CLAUDE.md, AGENTS.md, GEMINI.md, ...)", input)
	}

	// Silence per-adapter capability warnings during the multi-adapter
	// capture sweep below.
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	if err := adapters.ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
		return whyOutput{}, err
	}
	target, hit, exact, err := findEmittingAdapter(rel, resolved, b, cfg)
	if err != nil {
		return whyOutput{}, err
	}
	if target == "" || !exact {
		// Entry-point files (AGENTS.md, GEMINI.md, CONVENTIONS.md, ...) are
		// written by sync's entry-point distribution, not by any adapter,
		// so findEmittingAdapter never finds an exact (rel- or abs-path)
		// match for them. It can still surface a weak, best-effort basename
		// match against an unrelated per-target file that happens to share
		// the same leaf name at a different directory depth (e.g. Junie's
		// own `.junie/AGENTS.md` entry-point mirror versus the root
		// `AGENTS.md` this project-relative query means). Prefer the
		// entry-point resolution whenever it applies; only fall back to a
		// weak adapter match when this path inlines no rules at all.
		if report, ok := traceEntryPointFile(rel, cfg, b, projectRoot); ok {
			return report, nil
		}
		// Checked after the adapter lookup: a source directory can also
		// be an output directory (sources.rules: .claude/rules).
		if isSourceFile(rel, b) {
			return whyOutput{}, fmt.Errorf("%s is a source spec, not a generated file. Run `agnostic-ai explain %s` to see the files it writes", input, filepath.ToSlash(rel))
		}
		if target == "" {
			return whyOutput{}, whyNotTrackedError(input, projectRoot)
		}
	}

	adapter, err := adapters.Resolve(target)
	if err != nil {
		return whyOutput{}, err
	}
	sources, err := tracedSources(adapter, hit, b, cfg)
	if err != nil {
		return whyOutput{}, err
	}
	if mirror := adapters.SharedInstructionsMirror(target); mirror != "" && samePath(mirror, rel) {
		sources = append([]whySource{instructionsSource(len(sources) > 0)}, sources...)
	}

	out := whyOutput{
		Version:    "1",
		Command:    "why",
		File:       filepath.ToSlash(rel),
		Target:     target,
		Configured: slices.Contains(cfg.Targets, target),
		OutputKeys: outputKeysUsed(cfg, target, hit),
		Sources:    sources,
		LastSync:   lastSyncTimestamp(projectRoot),
	}
	return out, nil
}

// traceEntryPointFile reports where an entry-point file (CLAUDE.md,
// AGENTS.md, GEMINI.md, ...) comes from, following the blocks
// renderEntryPointFiles appends: the shared AGNOSTIC_AI.md body, then the
// rules inlined or imported for its readers, then the local extension.
// Returns false when rel is no configured target's managed entry point.
// The file is credited to the target whose rules it carries, or else to
// its first reader in `targets` order.
func traceEntryPointFile(rel string, cfg *config.Config, b spec.Bundle, projectRoot string) (whyOutput, bool) {
	var consumers []string
	for _, t := range cfg.Targets {
		p := adapters.EntryPointPath(cfg, t)
		if p == "" || p == adapters.AgnosticEntryPointPath || cfg.IsUnmanaged(p) ||
			adapters.LegacyRulesFileOwnsEntryPoint(cfg, t) || !samePath(p, rel) {
			continue
		}
		consumers = append(consumers, t)
	}
	if len(consumers) == 0 {
		return whyOutput{}, false
	}
	target := consumers[0]
	var rules []spec.Entry
	if inliners := pathRuleInliners(cfg, consumers); len(inliners) > 0 {
		target = inliners[0]
		rules = adapters.EntryPointRules(b, target, cfg).Rules
	} else if importer := pathRulesImporter(cfg, consumers); importer != "" {
		target = importer
		rules = adapters.EntryPointRules(b, importer, cfg).Rules
	} else if importer := pathLegacyRulesFileImporter(cfg, consumers); importer != "" {
		target = importer
		rules = adapters.EntryPointRules(b, importer, cfg).Rules
	}
	local, _ := adapters.ReadLocalInstructions()
	appended := len(rules) > 0 || local != "" || cfg.Sync.TargetOverview

	sources := []whySource{instructionsSource(appended)}
	var ruleSources []whySource
	for _, r := range rules {
		ruleSources = append(ruleSources, whyEntrySource(r, "section"))
	}
	sort.SliceStable(ruleSources, func(i, j int) bool { return ruleSources[i].Name < ruleSources[j].Name })
	sources = append(sources, ruleSources...)
	if local != "" {
		sources = append(sources, whySource{Kind: "instructions", Name: "AGNOSTIC_AI.md (local)", Path: filepath.ToSlash(adapters.ProjectLocalEntryPointPath), Mode: "section"})
	}
	return whyOutput{
		Version:    "1",
		Command:    "why",
		File:       filepath.ToSlash(rel),
		Target:     target,
		Configured: true,
		OutputKeys: nil,
		Sources:    sources,
		LastSync:   lastSyncTimestamp(projectRoot),
	}, true
}

// instructionsSource credits the shared AGNOSTIC_AI.md body: the whole
// file, or one section of it when something else lands there too.
func instructionsSource(shared bool) whySource {
	mode := "full"
	if shared {
		mode = "section"
	}
	return whySource{Kind: "instructions", Name: "AGNOSTIC_AI.md", Path: filepath.ToSlash(adapters.AgnosticEntryPointPath), Mode: mode}
}

// samePath compares two project-relative paths after cleaning, so an
// `outputs.<target>.file: ./docs/GEMINI.md` matches `docs/GEMINI.md`.
func samePath(a, b string) bool {
	return filepath.Clean(filepath.FromSlash(a)) == filepath.Clean(filepath.FromSlash(b))
}

// isSourceFile reports whether rel is a spec file, which sync reads
// rather than writes.
func isSourceFile(rel string, b spec.Bundle) bool {
	relSlash := filepath.ToSlash(rel)
	for _, e := range b.All() {
		if filepath.ToSlash(e.Path) == relSlash {
			return true
		}
	}
	return false
}

// normalizeInputPath returns the absolute path and the project-relative
// path of input. Symlinks in both input and projectRoot are followed, so
// a project reached through a link (macOS /tmp, a linked checkout) still
// yields a clean relative path. The file does not have to exist on disk:
// `why` traces from emitter output regardless.
func normalizeInputPath(input, projectRoot string) (absolute, relative string, err error) {
	if input == "" {
		return "", "", fmt.Errorf("why: missing file argument")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", input, err)
	}
	abs = resolveSymlinks(abs)
	rootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", projectRoot, err)
	}
	rootAbs = resolveSymlinks(rootAbs)
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		// Fall back to the user-supplied form when Rel fails (different drives).
		rel = input
	}
	return abs, rel, nil
}

// resolveSymlinks follows every symlink in the absolute path p. When p
// does not exist, it resolves the deepest existing ancestor and re-appends
// the missing tail, so an unsynced file under a linked directory resolves
// the same way as the directory.
func resolveSymlinks(p string) string {
	var tail []string
	for dir := p; ; dir = filepath.Dir(dir) {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		if filepath.Dir(dir) == dir {
			return p
		}
		tail = append(tail, filepath.Base(dir))
	}
}

// findEmittingAdapter runs every registered adapter in capture mode and
// returns the target name plus the captured file matching the user input,
// and whether that match was exact (project-relative or absolute path
// equality) as opposed to the basename-only fallback (best effort).
// Match order: project-relative path equality, then absolute path
// equality, then basename equality. Within one match kind, a target listed
// in cfg.Targets beats an unconfigured one, since several adapters share
// paths (`.agents/skills/` is written by codex, amp, and others).
func findEmittingAdapter(rel, abs string, b spec.Bundle, cfg *config.Config) (string, adapters.CapturedFile, bool, error) {
	type match struct {
		target     string
		file       adapters.CapturedFile
		score      int // 3 = rel match, 2 = abs match, 1 = basename match
		configured bool
	}
	var matches []match
	for _, name := range adapters.Names() {
		adapter, err := adapters.Resolve(name)
		if err != nil {
			continue
		}
		captured, err := captureEmit(adapter, b, cfg)
		if err != nil {
			return "", adapters.CapturedFile{}, false, fmt.Errorf("%s: %w", name, err)
		}
		for _, f := range captured {
			score := scoreCapturedMatch(f.Path, rel, abs)
			if score > 0 {
				matches = append(matches, match{target: name, file: f, score: score, configured: slices.Contains(cfg.Targets, name)})
			}
		}
	}
	if len(matches) == 0 {
		return "", adapters.CapturedFile{}, false, nil
	}
	// Highest score wins, then configured targets. Past that, prefer the
	// first registry entry to keep output deterministic.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		if matches[i].configured != matches[j].configured {
			return matches[i].configured
		}
		return matches[i].target < matches[j].target
	})
	best := matches[0]
	return best.target, best.file, best.score >= 2, nil
}

// scoreCapturedMatch ranks how closely a captured path matches the user
// input. Higher is better. Returns 0 for no match.
func scoreCapturedMatch(capturedPath, rel, abs string) int {
	cp := filepath.ToSlash(capturedPath)
	rp := filepath.ToSlash(rel)
	if cp == rp {
		return 3
	}
	if capturedAbs, err := filepath.Abs(capturedPath); err == nil {
		if filepath.ToSlash(capturedAbs) == filepath.ToSlash(abs) {
			return 2
		}
	}
	if filepath.Base(cp) == filepath.Base(rp) && filepath.Base(rp) != "" {
		return 1
	}
	return 0
}

// tracedSources determines which source specs contribute to hit by
// re-rendering the adapter with the bundle minus each entry and watching
// for the captured file to disappear (full ownership) or shrink
// (sectional contribution).
func tracedSources(adapter adapters.Adapter, hit adapters.CapturedFile, b spec.Bundle, cfg *config.Config) ([]whySource, error) {
	var sources []whySource
	for _, e := range b.All() {
		minus, err := captureEmit(adapter, bundleWithout(b, e), cfg)
		if err != nil {
			return nil, err
		}
		var matched *adapters.CapturedFile
		for i := range minus {
			if minus[i].Path == hit.Path {
				matched = &minus[i]
				break
			}
		}
		if matched == nil {
			sources = append(sources, whyEntrySource(e, "full"))
			continue
		}
		if matched.Content != hit.Content {
			sources = append(sources, whyEntrySource(e, "section"))
		}
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Kind != sources[j].Kind {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})
	return sources, nil
}

func whyEntrySource(e spec.Entry, mode string) whySource {
	r := entrySourceRef(e)
	return whySource{Kind: r.Kind, Name: r.Name, Path: r.Path, Mode: mode, Layer: r.Layer, Builtin: r.Builtin}
}

// outputKeysUsed walks the per-target Output struct and returns every
// non-empty `outputs.<target>.<key>` whose value appears as a substring
// of the resolved path. Best-effort: a literal scan, not a re-emit
// comparison.
func outputKeysUsed(cfg *config.Config, target string, hit adapters.CapturedFile) []string {
	o, ok := cfg.Outputs[target]
	if !ok {
		return nil
	}
	var keys []string
	pathSlash := filepath.ToSlash(hit.Path)
	v := reflect.ValueOf(o)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		val, ok := f.Interface().(string)
		if !ok || val == "" {
			continue
		}
		valSlash := filepath.ToSlash(val)
		if !strings.Contains(pathSlash, valSlash) {
			continue
		}
		tag := t.Field(i).Tag.Get("yaml")
		key := strings.Split(tag, ",")[0]
		if key == "" {
			key = t.Field(i).Name
		}
		keys = append(keys, fmt.Sprintf("outputs.%s.%s", target, key))
	}
	sort.Strings(keys)
	return keys
}

// lastSyncTimestamp returns the synced_at field from .agnostic-ai/.sync-state
// formatted as RFC3339, or nil when the state file is absent or unreadable.
func lastSyncTimestamp(projectRoot string) *string {
	data, err := os.ReadFile(stateFilePath(projectRoot))
	if err != nil {
		return nil
	}
	var s syncStateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	if s.SyncedAt.IsZero() {
		return nil
	}
	formatted := s.SyncedAt.UTC().Format(time.RFC3339)
	return &formatted
}

// whyNotTrackedError builds a clear "not tracked" error. When no sync has
// written the state file, suggest running sync first.
func whyNotTrackedError(input, projectRoot string) error {
	if ledgerMissing(projectRoot) {
		return fmt.Errorf("%s: no sync state found at %s. Run `agnostic-ai sync` first",
			input, filepath.ToSlash(stateFilePath(projectRoot)))
	}
	return fmt.Errorf("%s: not synced or not tracked by any adapter. Run `agnostic-ai sync` to (re)emit, or check that the path matches an adapter output", input)
}

func emitWhyJSON(cmd *cobra.Command, v whyOutput) error {
	if v.Sources == nil {
		v.Sources = []whySource{}
	}
	if v.OutputKeys == nil {
		v.OutputKeys = []string{}
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printWhyText(cmd *cobra.Command, r whyOutput) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s\n", r.File)
	if r.Configured {
		_, _ = fmt.Fprintf(out, "  adapter: %s\n", r.Target)
	} else {
		_, _ = fmt.Fprintf(out, "  adapter: %s (not configured)\n", r.Target)
	}
	if len(r.OutputKeys) > 0 {
		_, _ = fmt.Fprintf(out, "  output keys: %s\n", strings.Join(r.OutputKeys, ", "))
	} else {
		_, _ = fmt.Fprintln(out, "  output keys: (adapter defaults)")
	}
	if r.LastSync != nil {
		_, _ = fmt.Fprintf(out, "  last sync: %s\n", *r.LastSync)
	} else {
		_, _ = fmt.Fprintln(out, "  last sync: unknown")
	}
	if len(r.Sources) == 0 {
		_, _ = fmt.Fprintln(out, "  sources: (adapter emits this file unconditionally)")
		return
	}
	_, _ = fmt.Fprintln(out, "  sources:")
	for _, s := range r.Sources {
		mode := s.Mode
		if mode == "" {
			mode = "section"
		}
		path := s.Path
		if s.Builtin != nil {
			path = s.Builtin.String()
		}
		_, _ = fmt.Fprintf(out, "    [%s] %s (%s): %s\n", s.Kind, s.Name, path, mode)
	}
}
