package cli

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/spf13/cobra"
)

// lintCode describes one finding `lint` can raise, for `explain LINTnnn`.
// Keep in sync with the tables in docs/site/content/docs/cli-reference/check.md.
type lintCode struct {
	Severity lintSeverity
	Title    string
	Cause    string
	Fix      string
	Config   string
}

var lintCodeRe = regexp.MustCompile(`^LINT\d{3}$`)

func isLintCode(s string) bool { return lintCodeRe.MatchString(s) }

var lintCodes = map[string]lintCode{
	"LINT001": {
		Severity: lintWarn,
		Title:    "Empty spec",
		Cause:    "A spec has no body and no description, so every target receives an empty file or entry.",
		Fix:      "Write the spec's body or description, or delete the file.",
	},
	"LINT003": {
		Severity: lintError,
		Title:    "Duplicate spec name",
		Cause:    "Two specs of one kind share a `name`. The loader keeps one body and the other is lost. Hooks that share an event and matcher are fine.",
		Fix:      "Rename one of the specs, or merge them into one file.",
	},
	"LINT004": {
		Severity: lintWarn,
		Title:    "Spec no enabled target reads",
		Cause:    "No target in `targets` supports this spec's kind, so sync writes it nowhere. The finding lists the targets that do support it.",
		Fix:      "Enable one of the listed targets in agnostic-ai.yaml, or delete the spec.",
	},
	"LINT005": {
		Severity: lintWarn,
		Title:    "Hook matcher on an event without one",
		Cause:    "A hook sets `matcher`, but its event does not consume a matcher, so the native tool ignores it.",
		Fix:      "Drop the matcher, or use a tool-call event such as `PreToolUse` or `PostToolUse`.",
	},
	"LINT006": {
		Severity: lintError,
		Title:    "Unclosed frontmatter",
		Cause:    "Frontmatter opens with `---` and never closes, so the raw YAML is emitted as body.",
		Fix:      "Add the closing `---` line after the metadata.",
	},
	"LINT007": {
		Severity: lintWarn,
		Title:    "Likely misspelled frontmatter key",
		Cause:    "A frontmatter key is one edit away from a key agnostic-ai reads (`glob:` for `globs:`), so the setting is lost. A key some enabled target reads at the top level is not flagged.",
		Fix:      "Use the suggested key. Put target-native keys under `x-<target>:`.",
	},
	"LINT008": {
		Severity: lintError,
		Title:    "MCP server without command or url",
		Cause:    "A stdio MCP server lacks `command:`, or an `http`, `sse`, or `ws` one lacks `url:`. `x-<target>` cannot set either reserved field.",
		Fix:      "Set `command:` for stdio servers or `url:` for remote ones at the top level of the MCP spec.",
	},
	"LINT009": {
		Severity: lintWarn,
		Title:    "Permission that approves more than it says",
		Cause:    "An `allow` rule such as `Bash(git * main)` also matches a force push to main. A `deny` rule with the same shape blocks nothing in Claude Code. `ask` rules are skipped.",
		Fix:      "Write the exact value, or keep `*` at the end, as in `Bash(go test:*)`.",
	},
	"LINT010": {
		Severity: lintError,
		Title:    "Conditional rule in global specs",
		Cause:    "With `--global`, a rule has scope, path, glob, or target conditions, which `sync --global` rejects.",
		Fix:      "Remove the conditions, or move the rule into a project's `.agnostic-ai/`.",
	},
	"LINT011": {
		Severity: lintWarn,
		Title:    "Session-start context over budget",
		Cause:    "A target loads more words at session start than the budget allows: the entry-point file, always-on rules, skill and agent descriptions, and `@`-imported rules. Codex's scoped `AGENTS.md` chain is also checked against its byte budget, and targets with a published byte cap warn past it.",
		Fix:      "Scope rules with `globs`, move detail into skills, shorten descriptions, or raise the budget. Setting a review spec to `target: cursor` takes its text out of Codex's `AGENTS.md`.",
		Config:   "lint.instructions-words (default 2000), lint.codex-chain-bytes (default 32768)",
	},
	"LINT012": {
		Severity: lintWarn,
		Title:    "Description too long",
		Cause:    "A skill or agent description, or `x-<target>.description`, is longer than the budget. A skill description past 1024 characters, the Agent Skills limit, warns even under a raised budget.",
		Fix:      "Shorten the description and move detail into the body.",
		Config:   "lint.description-chars (default 1024)",
	},
	"LINT013": {
		Severity: lintError,
		Title:    "Malformed rule globs",
		Cause:    "A rule's `globs` or `x-<target>.globs` is neither a string nor a list of strings, so the rule loads in every session.",
		Fix:      "Write `globs` as one string or a YAML list of strings.",
	},
	"LINT014": {
		Severity: lintError,
		Title:    "Global effort a target cannot take",
		Cause:    "With `--global`, a settings `effort` is a value a target's user effort key cannot take, such as `max` for Claude or Copilot. `sync --global` drops it with a note.",
		Fix:      "Use an effort level every enabled target accepts, or set it per target.",
	},
	"LINT015": {
		Severity: lintWarn,
		Title:    "Spec names a target-native path",
		Cause:    "A spec body names another spec by a target-native path such as `.claude/skills/style/SKILL.md`, which other targets do not have.",
		Fix:      "Use the `.agnostic-ai/` source path the finding names.",
	},
	"LINT016": {
		Severity: lintError,
		Title:    "Invalid dev-commands entry",
		Cause:    "A `dev-commands` entry in an environment spec has no `name:` or `command:`, repeats a name, is not a mapping, sets a key no target reads, or gives `cwd`, `url`, `auto-port`, `port`, or `env` the wrong type. `x-claude` overrides are checked too.",
		Fix:      "Fix the entry the finding names so it has a unique name, a command, and correctly typed fields.",
	},
	"LINT017": {
		Severity: lintWarn,
		Title:    "gitignore.commit names a disabled target",
		Cause:    "A `gitignore.commit` entry such as `cursor:environments` names a target missing from `targets`, so it commits nothing.",
		Fix:      "Add the target to `targets`, or remove the entry.",
	},
	"LINT018": {
		Severity: lintWarn,
		Title:    "Skill sets scope",
		Cause:    "A skill's frontmatter sets `scope`, which has no effect. A skill's scope comes from its folder under `skills/`.",
		Fix:      "Move the skill folder under the scope it belongs to and drop `scope`. Use `workspaces` for extra Cursor copies.",
	},
	"LINT019": {
		Severity: lintWarn,
		Title:    "Claude Code body syntax other targets read as text",
		Cause:    "A skill or command line uses Claude Code body syntax (a `!` command substitution, a `!` code block, `$ARGUMENTS`, or `$0`, `$1`, ...) outside a `::target` fence, and an enabled target reads it as plain text.",
		Fix:      "Wrap the line in a `::claude` fence and give other targets their own wording, or rewrite it without the syntax.",
	},
	"LINT020": {
		Severity: lintWarn,
		Title:    "Rule folder and scope disagree",
		Cause:    "A rule sits in a folder that names a project directory, such as `rules/backend/`, but its `scope:` points outside it. The frontmatter wins.",
		Fix:      "Move the file to the folder for its scope, or drop `scope`.",
	},
	"LINT021": {
		Severity: lintWarn,
		Title:    "Bash permission without a matching Codex prefix",
		Cause:    "With Codex enabled, a supported Bash `allow` or `deny` rule has no explicit Codex prefix that covers it with the same effective decision, so Codex decides differently.",
		Fix:      "Add the Codex prefix rule the finding names, or narrow the portable rule.",
	},
	"LINT022": {
		Severity: lintWarn,
		Title:    "Protected path covers generated output",
		Cause:    "A settings `protected` path covers a file sync writes, such as `.claude/**`. Sync regenerates that file from its source spec, so the protection guards the wrong file.",
		Fix:      "Protect the spec under `.agnostic-ai/` instead.",
	},
	"LINT023": {
		Severity: lintError,
		Title:    "Invalid protected block",
		Cause:    "A settings `protected` block has no `paths`, a `decision` other than `ask` or `deny`, an unknown key, or a path outside the project or with a character class, brace, or negation. `sync` fails on the same block.",
		Fix:      "Fix the block the finding names.",
	},
	"LINT024": {
		Severity: lintWarn,
		Title:    "Unused coverage.accept entry",
		Cause:    "A `coverage.accept` entry matches no coverage note on one of its targets, for example because the target now supports the field.",
		Fix:      "Remove the entry, or remove that target from it.",
	},
	"LINT025": {
		Severity: lintWarn,
		Title:    "Model tier without an entry for a target",
		Cause:    "A `models` tier a spec names has no entry and no `default` for an enabled target that writes the spec, so the spec gets that tool's default model. Also raised for a tier named like a Claude model, such as `opus`.",
		Fix:      "Add the target or a `default` to the tier, or rename a tier that shadows a Claude model name.",
	},
	"LINT026": {
		Severity: lintWarn,
		Title:    "Claude model name reaches another target",
		Cause:    "A Claude model name reaches a target that cannot load it, through an agent's shared `model` or the `default` of a tier the agent names.",
		Fix:      "Write `model: {claude: <name>}`, or add the target to the tier.",
	},
	"LINT027": {
		Severity: lintError,
		Title:    "MCP value JSON cannot hold",
		Cause:    "An MCP spec holds a value JSON cannot hold, such as a YAML `.nan` or `.inf`, in the field the finding names. `sync` fails on it.",
		Fix:      "Replace the value with a finite number or a string.",
	},
}

type explainLintCodeOutput struct {
	Version  string `json:"version"`
	Command  string `json:"command"`
	Code     string `json:"code"`
	Title    string `json:"title"`
	Severity string `json:"severity"`
	Cause    string `json:"cause"`
	Fix      string `json:"fix"`
	Config   string `json:"config,omitempty"`
}

func runExplainLintCode(cmd *cobra.Command, code string, jsonOut bool) error {
	entry, ok := lintCodes[code]
	if !ok {
		return fmt.Errorf("unknown lint code: %s (see https://agnostic-ai.org/docs/cli-reference/check/ for the list)", code)
	}
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(explainLintCodeOutput{
			Version:  "1",
			Command:  "explain",
			Code:     code,
			Title:    entry.Title,
			Severity: entry.Severity.String(),
			Cause:    entry.Cause,
			Fix:      entry.Fix,
			Config:   entry.Config,
		})
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s: %s (%s)\n", code, entry.Title, entry.Severity)
	_, _ = fmt.Fprintf(out, "\nCause:\n  %s\n", entry.Cause)
	_, _ = fmt.Fprintf(out, "\nFix:\n  %s\n", entry.Fix)
	if entry.Config != "" {
		_, _ = fmt.Fprintf(out, "\nConfig:\n  %s\n", entry.Config)
	}
	return nil
}
