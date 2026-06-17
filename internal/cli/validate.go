package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newValidateCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate agnostic specs.",
		Example: `  # Parse every spec, list any issues
  agnostic-ai validate

  # Reconcile autofixable issues in source spec files
  agnostic-ai validate --fix`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, b, err := loadProject(".")
			if err != nil {
				return err
			}
			entries := b.All()
			cmd.Printf("loaded %d entries.\n", len(entries))
			// Declared-but-missing source dirs are reported even when no
			// specs loaded: an all-missing-sources config is exactly the
			// case where the warning matters most (#444).
			sourceIssues := lintMissingSources(".")
			if len(entries) == 0 {
				reportIssues(cmd, sourceIssues)
				cmd.PrintErrln(emptySpecsHint)
				return nil
			}
			issues := lintEntries(entries)
			issues = append(issues, lintHookEvents(entries, cfg.Targets)...)
			issues = append(issues, lintOrphanKinds(b, cfg.Targets)...)
			issues = append(issues, sourceIssues...)
			if !fix {
				reportIssues(cmd, issues)
				return nil
			}
			fixed, remaining, err := applyFixes(issues)
			if err != nil {
				return err
			}
			cmd.Printf("fixed %d issue(s).\n", fixed)
			if len(remaining) > 0 {
				cmd.Printf("%d issue(s) remain (not autofixable):\n", len(remaining))
				for _, i := range remaining {
					cmd.Printf("    %s: %s\n", i.Path, i.Message)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Apply autofixable issues by rewriting source spec files")
	return cmd
}

// lintEntries scans loaded specs for validation problems that are specific
// to individual entries. Markdown specs intentionally do not require
// `name:` in frontmatter: the spec format documents the field as optional
// and the loader derives a stable name from the file path.
func lintEntries(entries []spec.Entry) []validationIssue {
	var out []validationIssue
	return out
}

// reportIssues prints the issue list to the command's stdout. An empty
// list prints nothing so a clean validate run stays terse.
func reportIssues(cmd *cobra.Command, issues []validationIssue) {
	if len(issues) == 0 {
		return
	}
	cmd.Printf("%d issue(s) found:\n", len(issues))
	for _, i := range issues {
		marker := " "
		if i.Autofixable() {
			marker = "*"
		}
		cmd.Printf("  %s %s: %s\n", marker, i.Path, i.Message)
	}
	cmd.Printf("(* = autofixable; rerun with --fix)\n")
}

// validationIssue is one problem flagged by the linter. A non-zero
// kind makes the issue autofixable; the kind enum decides which fix
// routine `--fix` will run.
type validationIssue struct {
	Path    string
	Field   string
	Message string
	kind    issueKind
	entry   spec.Entry
}

// Autofixable reports whether `validate --fix` knows how to repair the
// issue without further user input.
func (i validationIssue) Autofixable() bool {
	return i.kind != issueNone
}

type issueKind int

const (
	issueNone issueKind = iota
	issueMissingName
)

// lintHookEvents flags hook specs whose `event:` value is unknown to
// every configured target. The union approach avoids false positives
// when a project enables several targets that each understand a
// disjoint subset (e.g. claude's `Notification` is fine when claude is
// in targets even though codex doesn't know it).
func lintHookEvents(entries []spec.Entry, targets []string) []validationIssue {
	var out []validationIssue
	allowed := unionEvents(targets)
	for _, e := range entries {
		if e.Kind != spec.KindHook {
			continue
		}
		event, _ := e.Meta["event"].(string)
		if event == "" {
			out = append(out, validationIssue{
				Path:    e.Path,
				Field:   "event",
				Message: "hook spec is missing required 'event:' field",
			})
			continue
		}
		if len(allowed) == 0 {
			continue // no hook-aware target enabled; the orphan-kind lint covers this
		}
		if _, ok := allowed[event]; ok {
			continue
		}
		out = append(out, validationIssue{
			Path:    e.Path,
			Field:   "event",
			Message: "unknown hook event " + quote(event) + " for enabled targets " + commaList(targets) + "; supported events: " + commaList(sortedKeys(allowed)),
		})
	}
	return out
}

// lintOrphanKinds reports each spec kind whose specs no enabled
// target consumes. Validation surfaces this once per kind, not per
// spec, so the message stays scannable.
func lintOrphanKinds(b spec.Bundle, targets []string) []validationIssue {
	type kindCount struct {
		kind  spec.Kind
		count int
		path  string
	}
	counts := []kindCount{
		{spec.KindHook, len(b.Hooks), firstPath(b.Hooks)},
		{spec.KindMCP, len(b.MCPs), firstPath(b.MCPs)},
		{spec.KindCommand, len(b.Commands), firstPath(b.Commands)},
		{spec.KindSettings, len(b.Settings), firstPath(b.Settings)},
		{spec.KindReview, len(b.Reviews), firstPath(b.Reviews)},
		{spec.KindEnvironment, len(b.Environments), firstPath(b.Environments)},
		{spec.KindIgnore, len(b.Ignores), firstPath(b.Ignores)},
	}
	enabled := setOf(targets...)
	var out []validationIssue
	for _, kc := range counts {
		if kc.count == 0 {
			continue
		}
		if anyTargetSupports(kc.kind, enabled) {
			continue
		}
		supporters := sortedKeys(targetsSupportingKind[kc.kind])
		out = append(out, validationIssue{
			Path:    kc.path,
			Field:   string(kc.kind),
			Message: pluralize(kc.count, string(kc.kind)) + " configured but no enabled target supports " + kindPlural(kc.kind) + ". Enable one of: " + commaList(supporters),
		})
	}
	return out
}

func unionEvents(targets []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range targets {
		for ev := range hookEventsByTarget[t] {
			out[ev] = struct{}{}
		}
	}
	return out
}

func anyTargetSupports(k spec.Kind, enabled map[string]struct{}) bool {
	for t := range targetsSupportingKind[k] {
		if _, ok := enabled[t]; ok {
			return true
		}
	}
	return false
}

func firstPath(entries []spec.Entry) string {
	if len(entries) == 0 {
		return ""
	}
	return entries[0].Path
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

func commaList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, ", ")
}

func quote(s string) string {
	return "\"" + s + "\""
}

func pluralize(n int, kind string) string {
	if n == 1 {
		return "1 " + kind + " spec"
	}
	return fmt.Sprintf("%d %s specs", n, kind)
}

// kindPlural returns the plural noun for a kind, leaving kinds that
// already end in "s" (e.g. "settings") unchanged so the message never
// reads "settingss".
func kindPlural(k spec.Kind) string {
	s := string(k)
	if strings.HasSuffix(s, "s") {
		return s
	}
	return s + "s"
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}

// applyFixes runs the autofix routine for every issue whose kind is
// non-zero. Returns the count fixed and the remaining (non-autofixable)
// issues so the caller can report them.
func applyFixes(issues []validationIssue) (fixed int, remaining []validationIssue, err error) {
	for _, i := range issues {
		if !i.Autofixable() {
			remaining = append(remaining, i)
			continue
		}
		switch i.kind {
		case issueMissingName:
			if err := fixInjectName(i.entry); err != nil {
				return fixed, remaining, fmt.Errorf("%s: %w", i.Path, err)
			}
			fixed++
		default:
			remaining = append(remaining, i)
		}
	}
	return fixed, remaining, nil
}

// fixInjectName rewrites a markdown spec to include a `name:` key in
// its frontmatter, deriving the value from the file path. The rest of
// the frontmatter and the body are preserved byte-for-byte.
func fixInjectName(e spec.Entry) error {
	data, err := os.ReadFile(e.Path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	derived := derivedSpecName(e)
	if derived == "" {
		return fmt.Errorf("cannot derive name from %s", e.Path)
	}

	patched, err := injectFrontmatterName(data, derived)
	if err != nil {
		return err
	}
	return os.WriteFile(e.Path, patched, 0o644)
}

// derivedSpecName picks the name used to backfill missing frontmatter.
// Skills nest under `<skill>/SKILL.md` so the parent directory is the
// authoritative name; everything else uses the bare filename.
func derivedSpecName(e spec.Entry) string {
	if e.Kind == spec.KindSkill && filepath.Base(e.Path) == "SKILL.md" {
		return filepath.Base(filepath.Dir(e.Path))
	}
	base := filepath.Base(e.Path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// injectFrontmatterName parses the existing frontmatter (creating an
// empty block when none is present), sets `name`, and re-serializes
// the document. yaml.v3 round-trips comments and field order for
// existing keys; new keys land at the end of the block.
func injectFrontmatterName(data []byte, name string) ([]byte, error) {
	const delim = "---"
	yamlBytes, body, hadFrontmatter := splitFrontmatter(data)

	var node yaml.Node
	if hadFrontmatter && len(bytes.TrimSpace(yamlBytes)) > 0 {
		if err := yaml.Unmarshal(yamlBytes, &node); err != nil {
			return nil, fmt.Errorf("parse existing frontmatter: %w", err)
		}
	}
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if node.Kind != yaml.DocumentNode || len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter root must be a mapping")
	}
	mapping := node.Content[0]

	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"}
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}
	updated := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "name" {
			mapping.Content[i+1] = valueNode
			updated = true
			break
		}
	}
	if !updated {
		mapping.Content = append([]*yaml.Node{keyNode, valueNode}, mapping.Content...)
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(delim + "\n")
	out.Write(buf.Bytes())
	out.WriteString(delim + "\n")
	out.WriteString(body)
	return out.Bytes(), nil
}

// splitFrontmatter mirrors spec.splitFrontmatter but returns the raw
// frontmatter bytes (rather than parsed meta) so the writer can
// preserve the existing layout. hadFrontmatter is false when the file
// has no leading `---` block.
func splitFrontmatter(data []byte) (yamlBytes []byte, body string, hadFrontmatter bool) {
	const delim = "---"
	if !bytes.HasPrefix(data, []byte(delim)) {
		return nil, string(data), false
	}
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte("\n"+delim))
	if idx < 0 {
		return nil, string(data), false
	}
	yamlBytes = rest[:idx]
	body = string(bytes.TrimLeft(rest[idx+len("\n"+delim):], "\n"))
	return yamlBytes, body, true
}
