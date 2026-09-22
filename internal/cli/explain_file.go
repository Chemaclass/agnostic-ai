package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// Applicability of one configured instruction to one source file.
const (
	contextAlways     = "always"         // loads with no file condition
	contextMatch      = "match"          // a file or directory selector covers the file
	contextNoMatch    = "no-match"       // a selector exists and misses the file
	contextModel      = "model-selected" // the agent decides from the description
	contextManual     = "manual"         // loads only when @-mentioned
	contextExcluded   = "excluded"       // target selection drops it for this target
	contextNotEmitted = "not-emitted"    // sync writes no output for it on this target
	contextUnknown    = "unknown"        // semantics this command cannot evaluate
)

// contextOrder sorts the report so what applies reads first.
var contextOrder = map[string]int{
	contextAlways: 0, contextMatch: 1, contextModel: 2, contextManual: 3,
	contextUnknown: 4, contextNoMatch: 5, contextExcluded: 6, contextNotEmitted: 7,
}

// fileContextNote is printed with every report. The command reads
// configuration, never a live session.
const fileContextNote = "Configured applicability, not a record of the model's active context. " +
	"Opening or reading a file does not guarantee the tool loads a matching instruction."

// fileContextItem is one configured instruction and its applicability.
type fileContextItem struct {
	Status   string `json:"status"`
	Source   string `json:"source"`
	Output   string `json:"output,omitempty"`
	Selector string `json:"selector,omitempty"`
	Reason   string `json:"reason"`
}

// explainFileOutput is the JSON envelope for `explain --file`.
type explainFileOutput struct {
	Version      string            `json:"version"`
	Command      string            `json:"command"`
	File         string            `json:"file"`
	Target       string            `json:"target"`
	Note         string            `json:"note"`
	Instructions []fileContextItem `json:"instructions"`
}

// fileContextTargets lists the targets whose discovery semantics
// `explain --file` models. Each one is verified against vendor docs.
var fileContextTargets = []string{"cursor"}

// sourceMarkerRE captures the spec path from the `<!-- source: ... -->`
// marker WriteSection stamps before each section of a merged document.
var sourceMarkerRE = regexp.MustCompile(`(?m)^[ \t]*<!--\s*source:\s*(\S+)\s*-->`)

// validateExplainInput rejects mixed or incomplete input modes before
// any project loading happens.
func validateExplainInput(args []string, file, target string) error {
	switch {
	case file != "" && len(args) > 0:
		return fmt.Errorf("--file cannot be combined with a spec or error code argument")
	case file != "" && target == "":
		return fmt.Errorf("--file requires --target (supported: %s)", strings.Join(fileContextTargets, ", "))
	case file == "" && target != "":
		return fmt.Errorf("--target applies only with --file")
	case file == "" && len(args) == 0:
		return fmt.Errorf("explain needs a spec path, an AAI-NNN error code, or --file")
	}
	return nil
}

// explainFile reports which configured instructions apply to one
// project file for target, using the output sync would plan. It writes
// nothing: every emission runs in capture mode.
func explainFile(input, target string, cfg *config.Config, b spec.Bundle, projectRoot string) (explainFileOutput, error) {
	if !slices.Contains(fileContextTargets, target) {
		return explainFileOutput{}, fmt.Errorf("explain --file: target %q is unsupported (supported: %s)", target, strings.Join(fileContextTargets, ", "))
	}
	if !slices.Contains(cfg.Targets, target) {
		return explainFileOutput{}, fmt.Errorf("explain --file: %s is not a configured target; add it to targets in agnostic-ai.yaml", target)
	}
	_, rel, err := normalizeInputPath(input, projectRoot)
	if err != nil {
		return explainFileOutput{}, err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return explainFileOutput{}, fmt.Errorf("%s: file is outside the project", input)
	}

	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	if err := adapters.ValidateScopedRules(cfg, b, cfg.Targets); err != nil {
		return explainFileOutput{}, err
	}
	docs, err := plannedAgentsDocs(cfg, b)
	if err != nil {
		return explainFileOutput{}, err
	}
	adapter, err := adapters.Resolve(target)
	if err != nil {
		return explainFileOutput{}, err
	}
	files, err := captureEmit(adapter, b, cfg)
	if err != nil {
		return explainFileOutput{}, fmt.Errorf("%s: %w", target, err)
	}

	included := b.For(target).Rules
	byName := make(map[string]spec.Entry, len(included))
	for _, r := range included {
		byName[r.Name] = r
	}
	reached := map[string]bool{}
	var items []fileContextItem
	for _, f := range files {
		if filepath.Ext(f.Path) != ".mdc" {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(f.Path), ".mdc")
		item := classifyMDC(f.Content, rel)
		item.Output = filepath.ToSlash(f.Path)
		if r, ok := byName[name]; ok {
			item.Source = filepath.ToSlash(r.Path)
			reached[item.Source] = true
		}
		items = append(items, item)
	}
	for _, d := range docs {
		items = append(items, agentsDocItems(d, rel, reached)...)
	}
	items = append(items, unreachedRuleItems(b.Rules, included, reached, target)...)

	sort.SliceStable(items, func(i, j int) bool {
		if contextOrder[items[i].Status] != contextOrder[items[j].Status] {
			return contextOrder[items[i].Status] < contextOrder[items[j].Status]
		}
		if items[i].Output != items[j].Output {
			return items[i].Output < items[j].Output
		}
		return items[i].Source < items[j].Source
	})
	return explainFileOutput{
		Version:      "1",
		Command:      "explain",
		File:         rel,
		Target:       target,
		Note:         fileContextNote,
		Instructions: items,
	}, nil
}

// agentsDoc is one planned AGENTS.md file and the targets writing it.
type agentsDoc struct {
	Path    string
	Content string
	Writers []string
}

// plannedAgentsDocs collects every AGENTS.md sync would write for the
// configured targets: root entry points and nested scope documents.
// Cursor reads AGENTS.md in the project root and in subdirectories, so
// a peer target's file reaches Cursor too.
func plannedAgentsDocs(cfg *config.Config, b spec.Bundle) ([]agentsDoc, error) {
	byPath := map[string]*agentsDoc{}
	add := func(p, content, writer string) {
		p = filepath.ToSlash(p)
		if path.Base(p) != "AGENTS.md" || cfg.IsUnmanaged(p) {
			return
		}
		d, ok := byPath[p]
		if !ok {
			d = &agentsDoc{Path: p, Content: content}
			byPath[p] = d
		}
		d.Writers = append(d.Writers, writer)
	}

	sess := adapters.NewSession()
	sess.StartCapture()
	body, err := resolveAgnosticBody(sess, cfg, false)
	sess.StopCapture()
	if err != nil {
		return nil, err
	}
	entryPoints, err := renderEntryPointFiles(cfg, b, cfg.Targets, body)
	if err != nil {
		return nil, err
	}
	for _, f := range entryPoints {
		for _, t := range cfg.Targets {
			if !adapters.LegacyRulesFileOwnsEntryPoint(cfg, t) && adapters.EntryPointPath(cfg, t) == f.Path {
				add(f.Path, f.Content, t)
			}
		}
	}
	for _, t := range cfg.Targets {
		a, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		captured, err := captureEmit(a, b, cfg)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range captured {
			add(f.Path, f.Content, t)
		}
	}
	docs := make([]agentsDoc, 0, len(byPath))
	for _, d := range byPath {
		docs = append(docs, *d)
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}

// agentsDocItems reports one AGENTS.md: the canonical entry-point body
// for a root file, then every spec section it carries. A root file has
// no activation metadata; a nested one covers its directory subtree.
func agentsDocItems(d agentsDoc, rel string, reached map[string]bool) []fileContextItem {
	dir := path.Dir(d.Path)
	writers := strings.Join(d.Writers, ", ")
	var status, selector, reason string
	if dir == "." {
		status, selector = contextAlways, "project-root AGENTS.md"
		reason = "written for " + writers + "; Cursor reads the project-root AGENTS.md, which has no activation metadata"
	} else {
		selector = "directory: " + dir
		if strings.HasPrefix(rel, dir+"/") {
			status = contextMatch
			reason = "written for " + writers + "; the file is under " + dir + "/ and Cursor reads AGENTS.md in subdirectories"
		} else {
			status = contextNoMatch
			reason = "written for " + writers + "; the file is outside " + dir + "/"
		}
	}
	item := func(source string) fileContextItem {
		return fileContextItem{Status: status, Source: source, Output: d.Path, Selector: selector, Reason: reason}
	}
	var items []fileContextItem
	if dir == "." {
		items = append(items, item(adapters.AgnosticEntryPointPath))
	}
	for _, m := range sourceMarkerRE.FindAllStringSubmatch(d.Content, -1) {
		reached[m[1]] = true
		items = append(items, item(m[1]))
	}
	return items
}

// unreachedRuleItems explains rules with no instruction reaching the
// target: dropped by target selection, or skipped during emission.
// reached is keyed by slash-separated source path.
func unreachedRuleItems(all, included []spec.Entry, reached map[string]bool, target string) []fileContextItem {
	in := make(map[string]bool, len(included))
	for _, r := range included {
		in[r.Path] = true
	}
	var items []fileContextItem
	for _, r := range all {
		source := filepath.ToSlash(r.Path)
		switch {
		case !in[r.Path]:
			items = append(items, fileContextItem{
				Status: contextExcluded,
				Source: source,
				Reason: "target selection (target, targets, target-exclude) leaves out " + target,
			})
		case !reached[source]:
			items = append(items, fileContextItem{
				Status: contextNotEmitted,
				Source: source,
				Reason: "sync writes no " + target + " instruction for this rule; `agnostic-ai sync` prints the coverage note",
			})
		}
	}
	return items
}

// cursorRuleFront is the `.mdc` frontmatter Cursor reads to decide when
// a rule loads (cursor.com/docs/rules, "Rule anatomy").
type cursorRuleFront struct {
	Description string `yaml:"description"`
	Globs       any    `yaml:"globs"`
	AlwaysApply *bool  `yaml:"alwaysApply"`
}

// classifyMDC applies Cursor's documented alwaysApply/description/globs
// matrix to one planned `.mdc` file:
//   - alwaysApply: true loads in every session; globs and description are ignored.
//   - alwaysApply: false with globs auto-attaches when a matching file is in context.
//   - alwaysApply: false with a description and no globs is model-selected.
//   - alwaysApply: false with neither loads only when @-mentioned.
//
// Anything outside that matrix is reported unknown instead of guessed.
func classifyMDC(content, rel string) fileContextItem {
	raw, _, ok := splitFrontmatter([]byte(content))
	var fm cursorRuleFront
	if !ok || yaml.Unmarshal(raw, &fm) != nil || fm.AlwaysApply == nil {
		return fileContextItem{Status: contextUnknown, Reason: "the planned .mdc has no readable alwaysApply frontmatter"}
	}
	if *fm.AlwaysApply {
		return fileContextItem{Status: contextAlways, Selector: "alwaysApply: true", Reason: "Cursor includes it in every chat session"}
	}
	if fm.Globs != nil {
		globs, isString := fm.Globs.(string)
		if !isString {
			return fileContextItem{Status: contextUnknown, Selector: "globs", Reason: "globs is not a comma-separated string"}
		}
		if strings.TrimSpace(globs) != "" {
			return classifyGlobs(globs, rel)
		}
	}
	if strings.TrimSpace(fm.Description) != "" {
		return fileContextItem{Status: contextModel, Selector: "description", Reason: "Cursor's agent reads the description and decides whether the rule is relevant"}
	}
	return fileContextItem{Status: contextManual, Selector: "@-mention", Reason: "Cursor includes it only when the rule is @-mentioned in chat"}
}

// classifyGlobs matches rel against Cursor's comma-separated globs.
// Brace sets, character classes, and negation have no documented Cursor
// semantics, so a pattern using them makes the result unknown unless
// another pattern already matches.
func classifyGlobs(globs, rel string) fileContextItem {
	selector := "globs: " + globs
	unknown := false
	for _, p := range splitGlobPatterns(globs) {
		if strings.ContainsAny(p, "{}[]!") {
			unknown = true
			continue
		}
		re, err := compileGlob(p)
		if err != nil {
			unknown = true
			continue
		}
		if re.MatchString(rel) {
			return fileContextItem{Status: contextMatch, Selector: selector, Reason: "pattern " + p + " matches the file; Cursor auto-attaches the rule when a matching file is in context"}
		}
	}
	if unknown {
		return fileContextItem{Status: contextUnknown, Selector: selector, Reason: "a pattern uses syntax Cursor does not document (braces, classes, or negation)"}
	}
	return fileContextItem{Status: contextNoMatch, Selector: selector, Reason: "no pattern matches the file"}
}

func runExplainForFile(cmd *cobra.Command, file, target string, jsonOut bool) error {
	cfg, bundle, err := loadProject(".")
	if err != nil {
		return err
	}
	report, err := explainFile(file, target, cfg, bundle, ".")
	if err != nil {
		return err
	}
	if jsonOut {
		if report.Instructions == nil {
			report.Instructions = []fileContextItem{}
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s (%s)\n", report.File, report.Target)
	_, _ = fmt.Fprintf(out, "%s\n\n", report.Note)
	if len(report.Instructions) == 0 {
		_, _ = fmt.Fprintln(out, "  (no configured instructions)")
	}
	for _, it := range report.Instructions {
		output := it.Output
		if output == "" {
			output = "(no " + report.Target + " output)"
		}
		_, _ = fmt.Fprintf(out, "  [%s] %s <- %s\n", it.Status, output, it.Source)
		detail := it.Reason
		if it.Selector != "" {
			detail = it.Selector + ": " + detail
		}
		_, _ = fmt.Fprintf(out, "      %s\n", detail)
	}
	return nil
}
