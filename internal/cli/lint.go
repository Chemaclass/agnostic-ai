package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// lintSeverity classifies the impact of a lint finding.
type lintSeverity int

const (
	lintWarn  lintSeverity = iota // non-zero exit only when --strict
	lintError                     // always non-zero exit
)

func (s lintSeverity) String() string {
	if s == lintError {
		return "error"
	}
	return "warn"
}

func (s lintSeverity) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// lintFinding is one semantic issue reported by the linter.
type lintFinding struct {
	Code     string       `json:"code"`
	Severity lintSeverity `json:"severity"`
	Path     string       `json:"path"`
	Message  string       `json:"message"`
}

func (f lintFinding) String() string {
	return fmt.Sprintf("%s [%s] %s: %s", f.Code, f.Severity, f.Path, f.Message)
}

func newLintCmd() *cobra.Command {
	var strict, global, asJSON, onlyFiles bool
	cmd := &cobra.Command{
		Use:   "lint [--files <path>...]",
		Short: "Run semantic lint checks on source specs beyond schema validation.",
		Long: "Checks for empty specs, duplicate names, dead specs (kinds not " +
			"supported by any enabled target), hooks whose event ignores their " +
			"matcher, unterminated frontmatter, and frontmatter keys that near-miss " +
			"a key agnostic-ai owns (allowed_tools vs tools), and allowed Bash rules " +
			"with a wildcard before the end of the command, spec bodies that name " +
			"another spec by one target's native path, skill and command lines with " +
			"Claude Code body syntax an enabled target reads as plain text, missing or conflicting " +
			"Codex command prefixes for Bash permissions, coverage.accept entries " +
			"that match no coverage note, MCP values JSON cannot hold, one tool's reference form in an MCP url or args, invalid protected paths and protected " +
			"paths that cover a file sync writes, Kiro agents whose resources omit the AGENTS.md that carries the rules, model tiers with no model for an enabled " +
			"target, Claude model names that reach another vendor's target, and warns when a " +
			"target's always-loaded instructions pass the lint.instructions-words " +
			"budget, the AGENTS.md chain Codex reads in a scope passes lint.codex-chain-bytes, " +
			"or a skill or agent description passes lint.description-chars. " +
			"With --global, it " +
			"also flags rules sync --global rejects and settings values a target cannot take. --json prints " +
			"the findings as JSON on stdout with the same exit status. --files reports only the " +
			"findings on the named files, read one per line from stdin for `-`; checks across " +
			"specs still load every spec. Exit code 1 on " +
			"error-severity findings, or on warn-severity findings when --strict " +
			"is set.",
		Example: `  # Lint all specs
  agnostic-ai lint

  # Treat warnings as errors (useful in CI)
  agnostic-ai lint --strict

  # Lint the global specs before sync --global writes them
  agnostic-ai lint --global

  # List finding codes in a script
  agnostic-ai lint --json | jq -r '.findings[].code'

  # Lint one spec, as an after-edit hook does
  agnostic-ai lint --files .agnostic-ai/skills/review/SKILL.md`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && !onlyFiles {
				return fmt.Errorf("lint takes paths only with --files")
			}
			if onlyFiles && global {
				return errs.Coded(errs.CodeFlagConflict, "--files cannot be combined with --global")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if onlyFiles {
				files, err := lintFileArgs(cmd.InOrStdin(), args)
				if err != nil {
					return err
				}
				if len(files) == 0 {
					return errors.New("--files needs at least one path; pass - to read them from stdin")
				}
				for _, f := range files {
					if _, err := os.Stat(f); err != nil {
						return fmt.Errorf("--files: %w", err)
					}
				}
				findings, err := lintFindingsForFiles(files)
				if err != nil {
					return err
				}
				if asJSON {
					return printLintJSON(cmd, findings, strict)
				}
				if len(findings) == 0 {
					cmd.Printf("ok — %d file(s) clean\n", len(files))
					return nil
				}
				printLintFindings(cmd, findings)
				return lintExitErr(findings, strict)
			}
			scope, err := loadCheckScope(global)
			if err != nil {
				return err
			}
			findings, err := lintScopeFindings(scope)
			if err != nil {
				return err
			}
			entries := scope.bundle.All()
			// A home or project with only AGNOSTIC_AI.md still loads it
			// every session, so its budget findings count without specs.
			empty := len(entries) == 0 && len(findings) == 0
			if empty {
				cmd.PrintErrln(scope.emptyHint())
			}
			if asJSON {
				return printLintJSON(cmd, findings, strict)
			}
			if empty {
				return nil
			}

			if len(findings) == 0 {
				cmd.Printf("ok — %d spec(s) clean\n", len(entries))
				return nil
			}

			printLintFindings(cmd, findings)
			return lintExitErr(findings, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors.")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print findings as JSON on stdout.")
	cmd.Flags().BoolVar(&onlyFiles, "files", false, "Report only findings on the paths given as arguments; - reads paths from stdin.")
	cmd.Flags().BoolVar(&global, "global", false, "Lint the global specs in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai) and its local/ layer, against the targets sync --global writes.")
	return cmd
}

func printLintFindings(cmd *cobra.Command, findings []lintFinding) {
	for _, f := range findings {
		cmd.Printf("%s\n", f)
	}
	cmd.Printf("\n%d finding(s): %d error(s), %d warning(s)\n",
		len(findings), countSeverity(findings, lintError), countSeverity(findings, lintWarn))
	cmd.Printf("Run `agnostic-ai explain %s` for a code's cause and fix.\n", findings[0].Code)
}

// lintFileArgs expands a `-` argument to the paths stdin lists, one per
// line, so a hook can pipe `hook paths` in without word splitting.
func lintFileArgs(stdin io.Reader, args []string) ([]string, error) {
	var files []string
	for _, a := range args {
		if a != "-" {
			files = append(files, a)
			continue
		}
		raw, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if line = strings.TrimRight(line, "\r"); line != "" {
				files = append(files, line)
			}
		}
	}
	return files, nil
}

// lintJSONOutput is the --json schema of `lint`. Findings share their
// shape with the lint list of `doctor --json`.
type lintJSONOutput struct {
	Version  string        `json:"version"`
	Command  string        `json:"command"`
	Findings []lintFinding `json:"findings"`
}

func printLintJSON(cmd *cobra.Command, findings []lintFinding, strict bool) error {
	if findings == nil {
		findings = []lintFinding{}
	}
	if err := writeIndentedJSON(cmd, lintJSONOutput{Version: "1", Command: "lint", Findings: slashLintPaths(findings)}); err != nil {
		return err
	}
	return lintExitErr(findings, strict)
}

// lintExitErr fails on any error finding, and on a warning with strict.
func lintExitErr(findings []lintFinding, strict bool) error {
	if countSeverity(findings, lintError) > 0 || (strict && countSeverity(findings, lintWarn) > 0) {
		return fmt.Errorf("lint failed")
	}
	return nil
}

// lintScopeFindings is every finding `lint` reports for a scope. doctor
// reads the same set, so it cannot pass while lint has findings.
func lintScopeFindings(scope checkScope) ([]lintFinding, error) {
	findings, _, err := lintScopeReport(scope)
	return findings, err
}

// lintScopeReport is lintScopeFindings plus the number of coverage notes
// coverage.accept matched, which doctor reports from the same emission.
func lintScopeReport(scope checkScope) ([]lintFinding, int, error) {
	budget, err := lintBudgetFindings(scope)
	if err != nil {
		return nil, 0, err
	}
	findings := collectLintFindings(scope.targets, scope.support, scope.bundle)
	if scope.global {
		findings = append(findings, lintGlobalRuleFindings(scope.bundle.Rules)...)
		findings = append(findings, lintGlobalSettingsFindings(scope.bundle.Settings, scope.targets)...)
	}
	findings = append(findings, budget...)
	findings = append(findings, lintGitignoreCommitTargets(scope.cfg)...)
	accepted := 0
	if !scope.global {
		permissions, err := lintCodexPermissions(scope.targets, scope.cfg, scope.bundle)
		if err != nil {
			return nil, 0, err
		}
		findings = append(findings, permissions...)
		coverage := matchCoverageAccept(scope.cfg, scope.bundle, scope.targets)
		findings = append(findings, lintCoverageAccept(coverage)...)
		accepted = coverage.accepted
		protected, err := lintProtectedPaths(scope)
		if err != nil {
			return nil, 0, err
		}
		findings = append(findings, protected...)
		findings = append(findings, lintKiroAgentResources(scope.cfg, scope.targets, scope.bundle)...)
	}
	findings = append(findings, lintGeminiHookVariables(scope.cfg, scope.targets, scope.bundle)...)
	configPath := config.ConfigFileName
	if scope.global {
		configPath = globalConfigPaths(scope.source)[0]
	}
	findings = append(findings, lintModels(scope.models, configPath, scope.targets, scope.support, scope.bundle)...)
	return findings, accepted, nil
}

// collectLintFindings runs every rule against a loaded bundle. Both `lint`
// and the LSP call it so the two cannot report different sets: before this
// existed the LSP silently lacked the newest rule.
func collectLintFindings(targets []string, support kindSupport, b spec.Bundle) []lintFinding {
	entries := b.All()

	// Shadowed entries lost a same-layer name clash and reach no target.
	// They are absent from All(), so LINT003 only sees the clash when they
	// are folded back in (#582).
	withShadowed := make([]spec.Entry, 0, len(entries)+len(b.Shadowed))
	withShadowed = append(withShadowed, entries...)
	withShadowed = append(withShadowed, b.Shadowed...)

	var findings []lintFinding
	findings = append(findings, lintEmptySpecs(entries)...)
	findings = append(findings, lintTodoDescriptions(entries)...)
	findings = append(findings, lintDuplicateNames(withShadowed)...)
	findings = append(findings, lintDeadSpecs(entries, targets, support)...)
	findings = append(findings, lintHookMatcherMisuse(b.Hooks)...)
	findings = append(findings, lintPortableHooks(b.Hooks, targets)...)
	findings = append(findings, lintUnterminatedFrontmatter(entries)...)
	findings = append(findings, lintNearMissKeys(entries, targets)...)
	findings = append(findings, lintMCPMissingRequiredField(b.MCPs)...)
	findings = append(findings, lintMCPNonJSONValues(b.MCPs)...)
	findings = append(findings, lintMCPToolRefs(targets, support, b.MCPs)...)
	findings = append(findings, lintDevCommands(b.Environments)...)
	findings = append(findings, lintSkillScopeKey(b.Skills)...)
	findings = append(findings, lintClaudeBodySyntax(b, targets, support)...)
	findings = append(findings, lintRuleFolderScope(b.Rules)...)
	findings = append(findings, lintMidWildcard(b.Settings)...)
	findings = append(findings, lintMalformedGlobs(b.Rules)...)
	findings = append(findings, lintNativeSpecPaths(b)...)
	findings = append(findings, lintSpecRefs(targets, support, b)...)
	return findings
}

// lintEmptySpecs flags specs with no body and no description (LINT001,
// warn). Settings and environment specs are fields alone, so they never
// have either.
func lintEmptySpecs(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		if e.Kind == spec.KindSettings || e.Kind == spec.KindEnvironment || hasHandler(e) {
			continue
		}
		body := strings.TrimSpace(e.Body)
		desc, _ := e.Meta["description"].(string)
		if body == "" && strings.TrimSpace(desc) == "" {
			out = append(out, lintFinding{
				Code:     "LINT001",
				Severity: lintWarn,
				Path:     e.Path,
				Message:  "empty spec — no body and no description",
			})
		}
	}
	return out
}

// hasHandler reports whether a hook or MCP spec names what it runs or
// connects to. Its YAML fields are its content, so no body or
// description makes it empty.
func hasHandler(e spec.Entry) bool {
	if e.Kind != spec.KindHook && e.Kind != spec.KindMCP {
		return false
	}
	for _, k := range []string{"command", "prompt", "server", "tool", "url", "agent"} {
		if v, ok := e.Meta[k]; ok && v != nil && v != "" {
			return true
		}
	}
	return false
}

// lintDuplicateNames flags two or more specs of the same kind sharing the
// same name, which causes later specs to silently overwrite earlier ones
// during layered loading (LINT003, error).
func lintDuplicateNames(entries []spec.Entry) []lintFinding {
	type key struct {
		kind spec.Kind
		name string
	}
	seen := map[key]string{} // key → first path
	var out []lintFinding
	for _, e := range entries {
		if e.Name == "" {
			continue // missing-name is reported by validate
		}
		k := key{e.Kind, e.Name}
		if prior, ok := seen[k]; ok {
			out = append(out, lintFinding{
				Code:     "LINT003",
				Severity: lintError,
				Path:     e.Path,
				Message: fmt.Sprintf(
					"duplicate %s name %q — also defined in %s; later spec shadows earlier one",
					e.Kind, e.Name, prior,
				),
			})
		} else {
			seen[k] = e.Path
		}
	}
	return out
}

// lintDeadSpecs flags individual specs whose kind is not supported by any
// enabled target (LINT004, warn). Unlike lintOrphanKinds in validate (which
// reports once per kind), this reports per-spec so the user can act on each
// file directly.
func lintDeadSpecs(entries []spec.Entry, targets []string, support kindSupport) []lintFinding {
	enabled := setOf(targets...)
	var out []lintFinding
	for _, e := range entries {
		if anyTargetSupports(e.Kind, enabled, support) {
			continue
		}
		supporters := sortedKeys(support[e.Kind])
		msg := fmt.Sprintf(
			"%s spec not consumed by any enabled target; targets that support %ss: %s",
			e.Kind, e.Kind, commaList(supporters),
		)
		out = append(out, lintFinding{
			Code:     "LINT004",
			Severity: lintWarn,
			Path:     e.Path,
			Message:  msg,
		})
	}
	return out
}

// lintHookMatcherMisuse flags hook specs whose event does not consume a
// matcher but still set one. The matcher is silently ignored by the native
// CLI, so the spec author likely intended a different event or should drop
// the matcher (LINT005, warn).
func lintHookMatcherMisuse(hooks []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		if event == "" || matcher == "" {
			continue
		}
		if _, ok := matcherAcceptingEvents[event]; ok {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT005",
			Severity: lintWarn,
			Path:     h.Path,
			Message: fmt.Sprintf(
				"matcher %q set but event %q does not consume a matcher; drop the matcher or use a tool-call event (e.g. PreToolUse, PostToolUse)",
				matcher, event,
			),
		})
	}
	return out
}

// lintUnterminatedFrontmatter flags specs that open a `---` block and never
// close it (LINT006, error).
//
// splitFrontmatter treats such a file as body-only, so the raw YAML survives
// as body text and every adapter writes it through verbatim. Nothing else
// catches this: the spec loads, validate passes, and sync exits 0 while
// emitting files whose frontmatter is structurally broken. Targets that write
// no frontmatter of their own end up with a single unterminated delimiter;
// targets that write their own block end up with a stray third one that opens
// a second block. Either way the agent silently never loads.
//
// A parsed block leaves Meta populated and strips the delimiters, so a body
// that still starts with `---` alongside empty Meta is the signature of the
// unterminated case.
func lintUnterminatedFrontmatter(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		if len(e.Meta) > 0 || !strings.HasPrefix(e.Body, "---") {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT006",
			Severity: lintError,
			Path:     e.Path,
			Message:  "frontmatter opens with `---` but is never closed; add the closing `---` or the block is emitted as body text",
		})
	}
	return out
}

// lintMCPMissingRequiredField flags an MCP spec with no `command:` on a
// stdio server, and none with no `url:` on a remote one (LINT008, error).
// `type` defaults to stdio, matching the spec format and every adapter.
//
// The check lives here rather than in the adapters because the entry is
// dead on every target, so the user should hear it once instead of per
// target. The two ways it dies split the fleet roughly in half. trae,
// antigravity and windsurf decline the entry and, before this rule, said
// nothing; claude, codex, cursor, gemini, copilot, amp, augment, factory,
// junie, kiro, opencode and warp write a server object carrying neither
// field, which no vendor schema accepts. Only continue reported it, and
// only because its loader throws on the entry and would lose the whole
// file (#739).
//
// Error rather than warn, for LINT006's reason: `validate` and `sync`
// both pass on such a spec, so nothing else tells the user the server
// will never start. `x-<target>` cannot rescue it either. Both field
// names are on every adapter's reserved list, so an override that sets
// one is dropped before it reaches the file.
// lintMCPNonJSONValues flags an MCP spec value JSON cannot hold, such as
// a YAML .nan (LINT027, error). Sync fails on it; validate reports it too.
func lintMCPNonJSONValues(mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range mcps {
		if field, value, ok := spec.NonJSONValue(e.Meta); ok {
			out = append(out, lintFinding{Code: "LINT027", Severity: lintError, Path: e.Path, Message: nonJSONValueMessage(e.Name, field, value)})
		}
	}
	return out
}

func lintMCPMissingRequiredField(mcps []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range mcps {
		// Every MCP builder drops a nameless entry before it reads the
		// transport, so there is no missing field to report. A spec
		// loaded from a file always has one, derived from the filename.
		if e.Name == "" {
			continue
		}
		transport, _ := e.Meta["type"].(string)
		if transport == "" {
			transport = "stdio"
		}
		field, verb := "url", "connect to"
		if transport == "stdio" {
			field, verb = "command", "start"
		} else if !remoteMCPTransports[transport] {
			continue // no known required field to name for this transport
		}
		if value, _ := e.Meta[field].(string); value != "" {
			continue
		}
		out = append(out, lintFinding{
			Code:     "LINT008",
			Severity: lintError,
			Path:     e.Path,
			Message: fmt.Sprintf(
				"%s MCP server %q has no `%s:`, so no target can %s it; add `%s:` or drop the spec",
				transport, e.Name, field, verb, field,
			),
		})
	}
	return out
}

// remoteMCPTransports is the set of transports the spec format documents
// as carrying `url`. A transport outside it (and outside stdio) has no
// documented required field, so LINT008 stays quiet rather than guess.
var remoteMCPTransports = map[string]bool{"http": true, "sse": true, "ws": true}

// midWildcardMessages are the LINT009 messages per list. In `allow`, a
// wildcard mid-command also matches options inserted at that spot, so
// `Bash(git * main)` approves `git push --force main`; Claude Code warns
// on the same shape at startup and the translating targets widen it
// silently. In `deny`, Claude Code matches that `*` literally, so the
// rule blocks nothing. Ask is skipped because widening it only prompts
// more.
var midWildcardMessages = map[string]string{
	"allow": "allow rule %q has a `*` before the end of the command, so it also approves any options inserted there; write the exact value or put `*` only at the end",
	"deny":  "deny rule %q has a `*` before the end of the command, which Claude Code matches literally there, so the rule blocks nothing; write the exact value or put `*` only at the end",
}

// lintMidWildcard flags an allowed or denied `Bash(...)` rule with a `*`
// anywhere but the end (LINT009, warn).
func lintMidWildcard(settings []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range settings {
		perms, _ := e.Meta["permissions"].(map[string]any)
		for _, list := range []string{"allow", "deny"} {
			rules, _ := perms[list].([]any)
			for _, raw := range rules {
				rule, _ := raw.(string)
				scope, arg, ok := spec.SplitPermissionRule(rule)
				if !ok || scope != "Bash" {
					continue
				}
				head := strings.TrimSuffix(strings.TrimSuffix(arg, "*"), ":")
				if !strings.Contains(head, "*") {
					continue
				}
				out = append(out, lintFinding{
					Code:     "LINT009",
					Severity: lintWarn,
					Path:     e.Path,
					Message:  fmt.Sprintf(midWildcardMessages[list], rule),
				})
			}
		}
	}
	return out
}

func countSeverity(findings []lintFinding, s lintSeverity) int {
	n := 0
	for _, f := range findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// nearMiss describes a frontmatter key that looks like one the tool
// understands but is read by nothing. Use names the replacement: an
// agnostic-ai key when one exists, otherwise the x-<target> form that
// actually reaches the tool.
type nearMiss struct {
	Use   string
	Owned bool
	// ReadBy names the only targets that read the key, none of them
	// configured here.
	ReadBy []string
}

// nearMissKeys is the near-miss set. Passthrough is the right default
// for genuine extensions, but a near miss of a key the tool owns is not
// an extension: it parses, emits, and does nothing. Nine phel-lang
// agents ran with full tool access because `allowed_tools:` passed
// validate, lint --strict, sync --check and doctor for months (#617).
//
// Two of these have no agnostic-ai equivalent at all. `maxTurns` and
// `disallowedTools` are Junie's own keys, reachable only under
// `x-junie`, so pointing at a bare agnostic-ai key would send users to
// one that does not exist.
var nearMissKeys = map[string]nearMiss{
	"allowed_tools":    {Use: "tools", Owned: true},
	"allowedTools":     {Use: "tools", Owned: true},
	"allowed-tools":    {Use: "tools", Owned: true},
	"model_name":       {Use: "model", Owned: true},
	"max_turns":        {Use: "x-junie.maxTurns"},
	"disallowed_tools": {Use: "x-junie.disallowedTools"},
}

// keyTypo reports a key one edit from a documented spec field (two for
// longer names) as a near miss of that field: `glob:` parses, emits, and
// leaves a rule meant for Go files applying everywhere. Keys any kind
// documents are never typos, and environments and settings are skipped:
// they pass their keys through to native files the tool owns.
func keyTypo(kind spec.Kind, key string, targets []string) (nearMiss, bool) {
	if kind == spec.KindEnvironment || kind == spec.KindSettings ||
		strings.HasPrefix(key, "x-") || slices.Contains(kindKeys(kind), key) {
		return nearMiss{}, false
	}
	readers, targetOnly := targetKeys[key]
	if targetOnly && slices.ContainsFunc(readers, func(t string) bool { return slices.Contains(targets, t) }) {
		return nearMiss{}, false
	}
	s := suggest.Name(key, kindKeys(kind))
	switch {
	case s == "":
		return nearMiss{}, false
	case targetOnly:
		return nearMiss{Use: s, ReadBy: readers}, true
	default:
		return nearMiss{Use: s, Owned: true}, true
	}
}

// lintNearMissKeys flags those keys at the top level of a spec's
// frontmatter (LINT007, warn). Warn rather than error because a
// target-native spelling can be legitimate: Junie really does document
// disallowedTools, just not at the top level. Keys already namespaced
// under x-<target> are deliberate and never flagged.
func lintNearMissKeys(entries []spec.Entry, targets []string) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		keys := make([]string, 0, len(e.Meta))
		for k := range e.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			miss, ok := nearMissKeys[k]
			if !ok {
				miss, ok = keyTypo(e.Kind, k, targets)
			}
			if !ok {
				continue
			}
			message := fmt.Sprintf("`%s:` is not a key agnostic-ai reads, so it has no effect.", k)
			advice := fmt.Sprintf("Use `%s:` instead.", miss.Use)
			switch {
			case len(miss.ReadBy) > 0:
				message = fmt.Sprintf("`%s:` is read only by %s, not a target here, so it has no effect.",
					k, strings.Join(miss.ReadBy, " and "))
				advice = fmt.Sprintf("Did you mean `%s:`?", miss.Use)
			case miss.Owned:
				advice = fmt.Sprintf("Did you mean `%s:`? If it is a target-native key, "+
					"move it under `x-<target>:` to keep it.", miss.Use)
			}
			out = append(out, lintFinding{
				Code:     "LINT007",
				Severity: lintWarn,
				Path:     e.Path,
				Message:  message + " " + advice,
			})
		}
	}
	return out
}
