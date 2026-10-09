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
		Title:    "Global setting a target cannot take",
		Cause:    "With `--global`, a settings `effort` or `permissions.default-mode` holds a value a target's user settings cannot take, such as effort `max` for Claude or Copilot. `sync --global` drops it with a note. The finding names the field and the values the target accepts.",
		Fix:      "Use one of the values the finding lists, or set the field per target.",
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
		Fix:      "Put the line between `::target claude` and `::end`, and give other targets their own `::target` block, or rewrite it without the syntax.",
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
		Title:    "Unused or unchecked coverage.accept entry",
		Cause:    "A `coverage.accept` entry matches no coverage note on one of its targets, for example because the target now supports the field. A target that fails to load or emit gets its own LINT024 naming the error, and its entries are not checked.",
		Fix:      "For an unused entry, remove it or that target from it. For an unchecked target, fix the error the finding names and run lint again before removing anything.",
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
	"LINT028": {
		Severity: lintWarn,
		Title:    "Tool reference form in an MCP url or args",
		Cause:    "An MCP spec's `url` or an `args` element holds a reference form only one tool reads, such as Cursor's `${env:NAME}`, OpenCode's `{env:NAME}`, or Continue's `${{ secrets.NAME }}`. Sync copies it as text, so each enabled tool that does not read that form gets the literal. Only the field sync writes for the transport is checked: `args` for stdio, `url` for a remote server. Older imports wrote these.",
		Fix:      "Write `${NAME}` so sync writes each tool's own form, or move the value under `x-<target>:` for the one tool that reads it.",
	},
	"LINT029": {
		Severity: lintWarn,
		Title:    "Kiro agent resources without AGENTS.md",
		Cause:    "A Kiro agent sets `x-kiro.resources` without `file://AGENTS.md`, and the always-on rules reach Kiro only through the `## Rules` block of `AGENTS.md`. Custom agents inherit `AGENTS.md` by default, but with Kiro's `chat.disableInheritingDefaultResources` setting on, that agent loads none of those rules.",
		Fix:      "Add `file://AGENTS.md` to the agent's `x-kiro.resources`, or ignore the warning if the setting stays off.",
	},
	"LINT030": {
		Severity: lintWarn,
		Title:    "Bare Gemini hook variable",
		Cause:    "A hook that emits to Gemini CLI has a command with a bare `$GEMINI_PROJECT_DIR`, `$GEMINI_CWD`, `$GEMINI_PLANS_DIR`, `$GEMINI_SESSION_ID`, or `$CLAUDE_PROJECT_DIR`. Gemini replaces each bare name with a shell-escaped value as text, before the shell runs, one name after another. Quotes, comments, heredocs, and other shell syntax around it, or a project path holding another such name, can turn the value into code. The braced `${NAME}` form is not replaced. See https://github.com/google-gemini/gemini-cli/blob/fb972b2f87fe7d5b06d37eac711490162d98de2c/packages/core/src/hooks/hookRunner.ts#L526-L531.",
		Fix:      "Write `\"${NAME}\"`: Gemini leaves the braced form to the shell, which reads the variable Gemini sets.",
	},
	"LINT031": {
		Severity: lintWarn,
		Title:    "Placeholder description",
		Cause:    "A spec's `description` still starts with `TODO`, the placeholder `agnostic-ai new` writes. Sync copies it to every target, where it shows in tool pickers and decides when a skill fires.",
		Fix:      "Replace the `description` with what the spec is for, and when it applies.",
	},
	"LINT032": {
		Severity: lintError,
		Title:    "Portable hook a target cannot run",
		Cause:    "A hook's `on:` or `match:` holds an unknown value, mixes with `event:` or `matcher:`, names an event an enabled target it reaches does not read the same way, or names a tool kind an enabled target it reaches has no tool for, so sync leaves it out there.",
		Fix:      "Use a value from the hooks page, keep one of `on:` and `event:`, or scope the hook away from that target with `target-exclude:`.",
	},
	"LINT033": {
		Severity: lintError,
		Title:    "Reference to an unknown agent or skill",
		Cause:    "A spec body holds `{{$AGENT:<name>}}` or `{{$SKILL:<name>}}`, and no agent or skill in the project has that name, or that agent or skill does not sync to a target the spec reaches. Sync would render an invocation phrase that points at nothing.",
		Fix:      "Use one of the names the finding lists, add the agent or skill, or scope both specs to the same targets.",
	},
	"LINT035": {
		Severity: lintError,
		Title:    "MCP env or headers value that is a literal",
		Cause:    "An MCP spec's `env` or `headers` value is neither a `${NAME}` reference nor marked `!literal`, so it may be a secret that sync writes into every tool's config. Empty values, numbers, booleans, and `x-<target>` blocks are not checked. `sync` stops on it.",
		Fix:      "Write a secret as `${NAME}` and set the variable, or mark a plain setting `NODE_ENV: !literal production`. `agnostic-ai migrate --only secrets` does both.",
	},
	"LINT034": {
		Severity: lintWarn,
		Title:    "Native hook event with an exact portable form",
		Cause:    "A hook's `event:` and `matcher:` have a portable `on:` and `match:` that give every enabled target the hook reaches the same native event and matcher, so the portable form syncs the same files. The finding names the values. It is raised for exactly the hooks `agnostic-ai migrate --only hooks` rewrites.",
		Fix:      "Run `agnostic-ai migrate --only hooks`, or write the `on:` and `match:` the finding names in place of `event:` and `matcher:`.",
	},
	"LINT037": {Title: "Native tool alias has a neutral capability", Severity: lintWarn, Cause: "A tool alias has an exact neutral form. This suggestion is enabled only by --suggest-capabilities.", Fix: "Use the suggested capability or run migrate --only capabilities."},
	"LINT038": {Severity: lintWarn, Title: "Bare capability covers every operation", Cause: "A bare capability in permissions.allow or permissions.ask now becomes a native permission for every operation it covers. Before permission capabilities, the lowercase spelling matched no tool.", Fix: "Scope shell, read, or edit, name one MCP tool, or restrict web access through a target-native permission field. write takes no path; edit(path) also covers edits."},
	"LINT039": {
		Severity: lintWarn,
		Title:    "Memory index too long",
		Cause:    "A shared memory index, `.agnostic-ai/memory/MEMORY.md` or `.agnostic-ai/local/memory/MEMORY.md`, has more than 100 lines, or the two indexes pass 6,000 bytes. Codex, Copilot, Cursor, Gemini CLI, Qoder, and Factory load memory through a session-start hook that keeps the first 6,000 bytes, so they never see the facts past it; the finding names them. Every other target loads both indexes whole every session.",
		Fix:      "Merge duplicate facts and drop stale ones. The `shared-memory` skill does this when asked to clean up memory.",
	},
	"LINT040": {
		Severity: lintError,
		Title:    "Memory index links a missing file",
		Cause:    "A line in a memory index links a fact file that does not exist, so a tool that follows it finds nothing.",
		Fix:      "Restore the file, or run `agnostic-ai memory index` to drop the line.",
	},
	"LINT041": {
		Severity: lintWarn,
		Title:    "Memory fact missing from the index",
		Cause:    "A fact file in a memory folder has no index line, so no tool loads it.",
		Fix:      "Run `agnostic-ai memory index` to add a line from the fact's frontmatter, or delete the file.",
	},
	"LINT042": {
		Severity: lintError,
		Title:    "Secret in a memory file",
		Cause:    "A line in a memory index or fact file looks like a credential, such as a token by its prefix, a URL password, or a `Bearer` token. Every tool reads memory as plain text, and project memory is committed. The finding names the line, never the value.",
		Fix:      "Remove the secret from the file and from Git history if it was committed, and rotate it.",
	},
	"LINT036": {
		Severity: lintError,
		Title:    "Capability sync cannot read",
		Cause:    "An agent's `can:` holds an unknown capability, a malformed `shell(<pattern>)` or `mcp:<server>`, or a value that is not a list, sits beside `tools:`, or sits under `x-<target>`. Or a settings permission rule is an unknown capability, a pattern on a capability that takes none, such as `write(.env)`, an empty pattern, or not a string, such as an unquoted rule holding `: `. Sync stops rather than write the agent or settings without their restriction.",
		Fix:      "Use a capability from the agents or settings page or a Claude Code name, and keep one of `can:` and `tools:`.",
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
