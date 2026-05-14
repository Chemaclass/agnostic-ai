package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
	Kind string `json:"kind"`
	Name string `json:"name"`
	Path string `json:"path"`
	Mode string `json:"mode"` // "full" or "section"
}

// whyOutput is the JSON envelope for `why --format json`.
type whyOutput struct {
	Version    string      `json:"version"`
	Command    string      `json:"command"`
	File       string      `json:"file"`
	Target     string      `json:"target"`
	OutputKeys []string    `json:"output_keys"`
	Sources    []whySource `json:"sources"`
	LastSync   *string     `json:"last_sync"`
}

func newWhyCmd() *cobra.Command {
	var format string
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
			if format == "json" {
				return emitWhyJSON(cmd, report)
			}
			printWhyText(cmd, report)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json.")
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

	// Silence per-adapter capability warnings during the multi-adapter
	// capture sweep below.
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)

	target, hit, err := findEmittingAdapter(rel, resolved, b, cfg)
	if err != nil {
		return whyOutput{}, err
	}
	if target == "" {
		return whyOutput{}, whyNotTrackedError(input, projectRoot)
	}

	adapter, err := adapters.Resolve(target)
	if err != nil {
		return whyOutput{}, err
	}
	sources, err := tracedSources(adapter, hit, b, cfg)
	if err != nil {
		return whyOutput{}, err
	}

	out := whyOutput{
		Version:    "1",
		Command:    "why",
		File:       filepath.ToSlash(rel),
		Target:     target,
		OutputKeys: outputKeysUsed(cfg, target, hit),
		Sources:    sources,
		LastSync:   lastSyncTimestamp(projectRoot),
	}
	return out, nil
}

// normalizeInputPath returns the absolute path and the project-relative
// path of input. Symlinks are followed when possible. The file does not
// have to exist on disk: `why` traces from emitter output regardless.
func normalizeInputPath(input, projectRoot string) (absolute, relative string, err error) {
	if input == "" {
		return "", "", fmt.Errorf("why: missing file argument")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", input, err)
	}
	if resolved, lerr := filepath.EvalSymlinks(abs); lerr == nil {
		abs = resolved
	}
	rootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", projectRoot, err)
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil {
		// Fall back to the user-supplied form when Rel fails (different drives).
		rel = input
	}
	return abs, rel, nil
}

// findEmittingAdapter runs every registered adapter in capture mode and
// returns the target name plus the captured file matching the user input.
// Match order: project-relative path equality, then absolute path
// equality, then basename equality (best effort).
func findEmittingAdapter(rel, abs string, b spec.Bundle, cfg *config.Config) (string, adapters.CapturedFile, error) {
	type match struct {
		target string
		file   adapters.CapturedFile
		score  int // 3 = rel match, 2 = abs match, 1 = basename match
	}
	var matches []match
	for _, name := range adapters.Names() {
		adapter, err := adapters.Resolve(name)
		if err != nil {
			continue
		}
		captured, err := captureEmit(adapter, b, cfg)
		if err != nil {
			return "", adapters.CapturedFile{}, fmt.Errorf("%s: %w", name, err)
		}
		for _, f := range captured {
			score := scoreCapturedMatch(f.Path, rel, abs)
			if score > 0 {
				matches = append(matches, match{target: name, file: f, score: score})
			}
		}
	}
	if len(matches) == 0 {
		return "", adapters.CapturedFile{}, nil
	}
	// Highest score wins. Within the same score, prefer the first registry
	// entry to keep output deterministic.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].target < matches[j].target
	})
	return matches[0].target, matches[0].file, nil
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
			sources = append(sources, whySource{
				Kind: string(e.Kind),
				Name: e.Name,
				Path: filepath.ToSlash(e.Path),
				Mode: "full",
			})
			continue
		}
		if matched.Content != hit.Content {
			sources = append(sources, whySource{
				Kind: string(e.Kind),
				Name: e.Name,
				Path: filepath.ToSlash(e.Path),
				Mode: "section",
			})
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

// whyNotTrackedError builds a clear "not tracked" error. When the sync
// state file is absent altogether, suggest running sync first.
func whyNotTrackedError(input, projectRoot string) error {
	if _, err := os.Stat(stateFilePath(projectRoot)); errors.Is(err, fs.ErrNotExist) {
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
	_, _ = fmt.Fprintf(out, "  adapter: %s\n", r.Target)
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
		_, _ = fmt.Fprintf(out, "    [%s] %s (%s): %s\n", s.Kind, s.Name, s.Path, mode)
	}
}
