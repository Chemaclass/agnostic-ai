package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// compareCoverage states what `compare` inspects, so a clean report is
// never read as "the whole project ports".
const compareCoverage = "agent fields and rule scope/activation only; " +
	"skills, hooks, MCP servers, commands, settings, reviews, environments, and ignore files are not compared"

// compareCaveat keeps "preserved" from reading as a behavior guarantee.
const compareCaveat = "preserved means the field is written under the same key; it does not prove the tools behave the same"

type compareStatus string

const (
	// statusPreserved: the target writes the field under its own name.
	statusPreserved compareStatus = "preserved"
	// statusTranslated: the field changes the output, under another name
	// or location, or only partly survives.
	statusTranslated compareStatus = "translated"
	// statusUnsupported: the target has no home for the field or kind.
	statusUnsupported compareStatus = "unsupported"
	// statusExcluded: the spec does not reach the target at all.
	statusExcluded compareStatus = "excluded"
	// statusUnknown: the emission gives no evidence either way.
	statusUnknown compareStatus = "unknown"
)

// ruleActivationFields are the portable rule fields that decide where and
// when a rule loads. Other rule fields carry text, not activation.
var ruleActivationFields = map[string]bool{
	"scope": true, "paths": true, "globs": true, "alwaysApply": true,
}

// compareSkippedAgentFields name the agent and route it; they are not
// behavior a target can keep or lose.
var compareSkippedAgentFields = map[string]bool{
	"name": true, "target": true, "targets": true, "target-exclude": true, "targets-exclude": true,
}

type compareResult struct {
	Target string        `json:"target"`
	Status compareStatus `json:"status"`
	Paths  []string      `json:"paths,omitempty"`
	Reason string        `json:"reason,omitempty"`
	Next   string        `json:"next,omitempty"`
}

type compareField struct {
	Field   string          `json:"field"`
	Differs bool            `json:"differs"`
	Results []compareResult `json:"results"`
}

type compareNote struct {
	Target string `json:"target"`
	Text   string `json:"text"`
}

type compareSpec struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Path   string         `json:"path"`
	Fields []compareField `json:"fields"`
	Notes  []compareNote  `json:"notes,omitempty"`
}

// compareOutput is the JSON envelope for `compare --json`.
type compareOutput struct {
	Version     string        `json:"version"`
	Command     string        `json:"command"`
	Targets     []string      `json:"targets"`
	Coverage    string        `json:"coverage"`
	Caveat      string        `json:"caveat"`
	Specs       []compareSpec `json:"specs"`
	Fields      int           `json:"fields"`
	Differences int           `json:"differences"`
}

func newCompareCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "compare <target> <target>",
		Short: "Compare how two targets represent agent fields and rule activation.",
		Long: "Emits every agent and every rule with scope or activation fields " +
			"to both targets in memory, then reports per field whether each " +
			"target preserves, translates, drops, or never receives it. " +
			"Uses the project's specs, output options, and x-<target> " +
			"overrides. Writes nothing.\n\n" +
			"Coverage is limited to agent fields and rule scope/activation. " +
			"A preserved field is written under the same key; that does not " +
			"prove both tools behave the same.",
		Example: `  # Before switching from Claude Code to Cursor
  agnostic-ai compare claude cursor

  # Machine-readable, for scripts
  agnostic-ai compare claude codex --json`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) >= 2 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return adapters.Names(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCompareTargets(args); err != nil {
				return err
			}
			cfg, bundle, err := loadProject(".")
			if err != nil {
				return err
			}
			out, err := compareTargets(cfg, bundle, args)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			writeCompareReport(cmd.OutOrStdout(), out)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON for scripts.")
	return cmd
}

// checkCompareTargets accepts exactly two distinct built-in targets.
// External adapters are refused: their behavior is not ours to report.
func checkCompareTargets(targets []string) error {
	for _, t := range targets {
		if _, ok := adapters.Get(t); !ok {
			return fmt.Errorf("unknown target %q: compare accepts built-in targets (%s)", t, strings.Join(adapters.Names(), ", "))
		}
	}
	if targets[0] == targets[1] {
		return fmt.Errorf("compare needs two different targets, got %q twice", targets[0])
	}
	return nil
}

// compareTargets builds the report. Unsupported fields are report data,
// so the on-unsupported policy is forced to warn: an `error` policy would
// otherwise turn the first gap into a failed command.
//
// Cross-target reader checks (sync's ValidateScopedRules) are skipped on
// purpose: they guard targets written side by side, and compare writes
// nothing. An invalid spec still fails, from inside the emission.
func compareTargets(cfg *config.Config, b spec.Bundle, targets []string) (compareOutput, error) {
	view := *cfg
	view.OnUnsupported = "warn"
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	adapters.DrainNotes()

	out := compareOutput{
		Version: "1", Command: "compare", Targets: targets,
		Coverage: compareCoverage, Caveat: compareCaveat, Specs: []compareSpec{},
	}
	for _, e := range compareEntries(b) {
		fields := comparedFields(e)
		if len(fields) == 0 {
			continue
		}
		s := compareSpec{Kind: string(e.Kind), Name: e.Name, Path: filepath.ToSlash(e.Path)}
		perTarget := make([]map[string]compareResult, len(targets))
		for i, t := range targets {
			results, notes, err := classifyEntry(&view, e, fields, t)
			if err != nil {
				return compareOutput{}, fmt.Errorf("%s: %s: %w", t, e.Path, err)
			}
			perTarget[i] = results
			s.Notes = append(s.Notes, notes...)
		}
		for _, f := range fields {
			cf := compareField{Field: f}
			for i := range targets {
				cf.Results = append(cf.Results, perTarget[i][f])
			}
			cf.Differs = cf.Results[0].Status != cf.Results[1].Status
			if cf.Differs {
				out.Differences++
			}
			out.Fields++
			s.Fields = append(s.Fields, cf)
		}
		out.Specs = append(out.Specs, s)
	}
	return out, nil
}

// compareEntries returns agents then rules, each sorted by source path.
func compareEntries(b spec.Bundle) []spec.Entry {
	byPath := func(es []spec.Entry) []spec.Entry {
		out := append([]spec.Entry(nil), es...)
		sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		return out
	}
	return append(byPath(b.Agents), byPath(b.Rules)...)
}

// comparedFields lists the fields to report for e in source order. A
// rule's directory-derived scope counts as its `scope` field.
func comparedFields(e spec.Entry) []string {
	keys := append([]string(nil), e.MetaKeys...)
	var extra []string
	for k := range e.Meta {
		if !containsString(keys, k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)

	var out []string
	if e.Kind == spec.KindRule && e.Scope != "" {
		out = append(out, "scope")
	}
	for _, k := range keys {
		if _, ok := e.Meta[k]; !ok || strings.HasPrefix(k, "x-") || containsString(out, k) {
			continue
		}
		switch e.Kind {
		case spec.KindAgent:
			if !compareSkippedAgentFields[k] {
				out = append(out, k)
			}
		case spec.KindRule:
			if ruleActivationFields[k] {
				out = append(out, k)
			}
		}
	}
	return out
}

// classifyEntry reports, for one target, what happens to each field of
// e. It emits e alone, then e minus one field at a time, and reads the
// difference together with the coverage notes the adapter raised. The
// adapters stay the only source of truth: no capability table here.
func classifyEntry(cfg *config.Config, e spec.Entry, fields []string, target string) (map[string]compareResult, []compareNote, error) {
	results := make(map[string]compareResult, len(fields))
	fill := func(r compareResult) {
		r.Target = target
		for _, f := range fields {
			results[f] = r
		}
	}
	src := filepath.ToSlash(e.Path)
	if !e.EmitsTo(target) {
		fill(compareResult{
			Status: statusExcluded,
			Reason: "the spec's target filter skips " + target,
			Next:   "edit target, targets, target-exclude, or targets-exclude in " + src,
		})
		return results, nil, nil
	}
	adapter, _ := adapters.Get(target)
	base, notes, err := captureEntry(adapter, cfg, target, e)
	if err != nil {
		return nil, nil, err
	}
	if len(base) == 0 {
		fill(noOutputResult(notes, e.Kind, target))
		return results, nil, nil
	}
	var specNotes []compareNote
	for _, n := range notes {
		switch n.Shape {
		case adapters.NoteSurface:
			specNotes = append(specNotes, compareNote{Target: target, Text: fmt.Sprintf("not %s (%s)", n.Surface, n.Reason)})
		case adapters.NoteGap:
			specNotes = append(specNotes, compareNote{Target: target, Text: n.Reason})
		}
	}
	for _, f := range fields {
		r := compareResult{Target: target}
		variant, _, err := captureEntry(adapter, cfg, target, withoutField(e, f, target))
		if err != nil {
			r.Status = statusUnknown
			r.Reason = "cannot emit the spec without this field: " + err.Error()
			results[f] = r
			continue
		}
		changed, landed := fieldEffect(base, variant)
		note, noted := fieldNote(notes, f)
		keyed, verbatim := pathsWithKey(base, f, resolvedValue(e, f, target))
		switch {
		case noted && !changed:
			r.Status, r.Reason = statusUnsupported, note
		case noted:
			r.Status, r.Paths, r.Reason = statusTranslated, landed, "partly: "+note
		case len(keyed) > 0 && verbatim:
			r.Status, r.Paths = statusPreserved, keyed
		case len(keyed) > 0:
			r.Status, r.Paths = statusTranslated, keyed
			r.Reason = "same key, rewritten values"
		case changed:
			r.Status, r.Paths = statusTranslated, landed
		default:
			r.Status = statusUnsupported
			r.Reason = "no " + target + " output carries it"
		}
		results[f] = r
	}
	return results, specNotes, nil
}

// noOutputResult explains a spec the target emits nothing for, from the
// notes the adapter raised while skipping it.
func noOutputResult(notes []adapters.Note, kind spec.Kind, target string) compareResult {
	for _, n := range notes {
		switch n.Shape {
		case adapters.NoteUnsupportedKind:
			return compareResult{Target: target, Status: statusUnsupported, Reason: fmt.Sprintf("%s has no native %s support", target, kind)}
		case adapters.NoteGap:
			r := compareResult{Target: target, Status: statusExcluded, Reason: n.Reason}
			if strings.HasPrefix(n.Reason, "outputs.") {
				r.Reason = "reaches " + target + " only via " + n.Reason
				r.Next = "set " + n.Reason + " in agnostic-ai.yaml"
			}
			return r
		}
	}
	return compareResult{Target: target, Status: statusUnknown, Reason: target + " writes no output for this spec and reports no reason"}
}

// captureEntry emits e alone the way `render` does, adapter output plus
// any entry-point rule block, and returns the notes raised meanwhile.
func captureEntry(a adapters.Adapter, cfg *config.Config, target string, e spec.Entry) ([]adapters.CapturedFile, []adapters.Note, error) {
	single := singleEntryBundle(e)
	files, err := captureEmit(a, single, cfg)
	notes := adapters.DrainNotes()
	if err != nil {
		return nil, nil, err
	}
	if extra, ok := entryPointRuleFile(cfg, target, e, single); ok {
		files = append(files, extra)
	}
	return files, notes, nil
}

// withoutField returns a copy of e with field removed, both at the top
// level and from the target's own x-<target> override, so an override
// cannot keep the field alive in the variant.
func withoutField(e spec.Entry, field, target string) spec.Entry {
	meta := make(map[string]any, len(e.Meta))
	for k, v := range e.Meta {
		if k != field {
			meta[k] = v
		}
	}
	xKey := "x-" + target
	if override, ok := meta[xKey].(map[string]any); ok {
		trimmed := make(map[string]any, len(override))
		for k, v := range override {
			if k != field {
				trimmed[k] = v
			}
		}
		meta[xKey] = trimmed
	}
	e.Meta = meta
	var keys []string
	for _, k := range e.MetaKeys {
		if k != field {
			keys = append(keys, k)
		}
	}
	e.MetaKeys = keys
	if field == "scope" {
		e.Scope = ""
	}
	return e
}

// fieldNote returns the reason of the first field no-op note for field.
func fieldNote(notes []adapters.Note, field string) (string, bool) {
	for _, n := range notes {
		if n.Shape == adapters.NoteField && n.Field == field {
			return n.Reason, true
		}
	}
	return "", false
}

// fieldEffect compares the capture with a field (base) to the capture
// without it (variant). It reports whether any output differs and lists,
// sorted, the base paths that carry the difference: where the field lands.
func fieldEffect(base, variant []adapters.CapturedFile) (bool, []string) {
	without := make(map[string]string, len(variant))
	for _, f := range variant {
		without[filepath.ToSlash(f.Path)] = f.Content
	}
	var landed []string
	for _, f := range base {
		p := filepath.ToSlash(f.Path)
		if other, ok := without[p]; !ok || other != f.Content {
			landed = append(landed, p)
		}
		delete(without, p)
	}
	sort.Strings(landed)
	return len(landed) > 0 || len(without) > 0, landed
}

// pathsWithKey lists, sorted, the captured files that declare field as a
// key: a YAML, TOML, or JSON key line, or a TOML table header. Markdown
// files are searched in their frontmatter only, so a body line that
// happens to start with the word never counts. verbatim reports whether
// every such file also spells out every scalar in value, so a key kept
// with rewritten values (tool names mapped to the target's own) does not
// pass as preserved.
func pathsWithKey(files []adapters.CapturedFile, field string, value any) (keyed []string, verbatim bool) {
	q := regexp.QuoteMeta(field)
	re := regexp.MustCompile(`(?m)^\s*(?:["']?` + q + `["']?\s*[:=]|\[` + q + `[\].])`)
	want := scalarStrings(value)
	verbatim = true
	for _, f := range files {
		content := f.Content
		if ext := filepath.Ext(f.Path); ext == ".md" || ext == ".mdc" {
			content = frontmatterBlock(content)
		}
		if !re.MatchString(content) {
			continue
		}
		keyed = append(keyed, filepath.ToSlash(f.Path))
		for _, w := range want {
			if !strings.Contains(content, w) {
				verbatim = false
			}
		}
	}
	sort.Strings(keyed)
	return keyed, verbatim
}

// scalarStrings spells out the scalars of a frontmatter value: the value
// itself, or each element of a list. Maps return nil: their layout varies
// too much per format to check by substring.
func scalarStrings(value any) []string {
	switch v := value.(type) {
	case nil, map[string]any:
		return nil
	case []any:
		var out []string
		for _, x := range v {
			out = append(out, scalarStrings(x)...)
		}
		return out
	case []string:
		return v
	default:
		return []string{fmt.Sprint(v)}
	}
}

// resolvedValue returns field's value as target sees it: the target's
// x-<target> override when it sets the field, the top-level value
// otherwise.
func resolvedValue(e spec.Entry, field, target string) any {
	if override, ok := e.Meta["x-"+target].(map[string]any); ok {
		if v, ok := override[field]; ok {
			return v
		}
	}
	return e.Meta[field]
}

// frontmatterBlock returns the leading `---` delimited block of a
// Markdown document, or "" when there is none.
func frontmatterBlock(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return ""
	}
	return content[4 : 4+end]
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// writeCompareReport renders the human report: coverage boundary first,
// then one block per spec with each field's per-target result.
func writeCompareReport(w io.Writer, out compareOutput) {
	a, b := out.Targets[0], out.Targets[1]
	_, _ = fmt.Fprintf(w, "compare %s %s\n", a, b)
	_, _ = fmt.Fprintf(w, "coverage: %s\n", compareCoverage)
	_, _ = fmt.Fprintf(w, "note: %s\n", compareCaveat)
	if len(out.Specs) == 0 {
		_, _ = fmt.Fprintln(w, "\nno agents or scoped rules to compare")
		return
	}
	width := max(len(a), len(b))
	for _, s := range out.Specs {
		_, _ = fmt.Fprintf(w, "\n%s %s  %s\n", s.Kind, s.Name, s.Path)
		for _, f := range s.Fields {
			label := f.Field
			if f.Differs {
				label += " (differs)"
			}
			_, _ = fmt.Fprintf(w, "  %s\n", label)
			for _, r := range f.Results {
				detail := strings.Join(r.Paths, ", ")
				if r.Reason != "" {
					detail = strings.TrimSpace(strings.Join([]string{detail, r.Reason}, " "))
				}
				_, _ = fmt.Fprintf(w, "    %-*s  %-11s  %s\n", width, r.Target, r.Status, detail)
				if r.Next != "" {
					_, _ = fmt.Fprintf(w, "    %-*s  %-11s  next: %s\n", width, "", "", r.Next)
				}
			}
		}
		for _, n := range s.Notes {
			_, _ = fmt.Fprintf(w, "  note: %s: %s\n", n.Target, n.Text)
		}
	}
	_, _ = fmt.Fprintf(w, "\n%d of %d fields differ between %s and %s\n", out.Differences, out.Fields, a, b)
}
