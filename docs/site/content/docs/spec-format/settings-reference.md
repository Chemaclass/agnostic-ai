+++
title = "Settings reference"
description = "Per-tool details for settings: session retention, voice dictation, effort, support by target, and native keys."
weight = 71

[extra]
group = "Reference"
+++

# Settings reference

See [Settings](@/docs/spec-format/settings.md) for the shared fields.

## Effort by target {#effort-by-target}

`effort` reaches the tools below. A value the tool does not accept, or `effort` on any other tool, raises a sync note instead. `x-<target>` wins, so `x-claude.effortLevel` overrides the portable value. `import` fills `effort` from these keys when the tool accepts the value and no other settings spec for that tool sets it; otherwise the value goes under `x-<target>`. Kimi import does not read effort.

| Target | Native key | Accepted values |
|---|---|---|
| Augment | `defaultThinkingLevel` in `.augment/settings.json`, read by Auggie 1.x | `off`, `minimal`, `low`, `medium`, `high`, `xhigh` |
| Claude Code | `effortLevel` in `.claude/settings.json` | `low`, `medium`, `high`, `xhigh` |
| Copilot | `effortLevel` in `.github/copilot/settings.json` | `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; each model accepts a subset |
| Codex | `model_reasoning_effort` in `.codex/config.toml` | any string; `outputs.codex.config.model-reasoning-effort` and the captured overlay win |
| CodeBuddy | `reasoningEffort` in `.codebuddy/settings.json` | `minimal`, `low`, `medium`, `high`, `xhigh`, `max` |
| Factory | `reasoningEffort` in `.factory/settings.json` | `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; each model accepts a subset |
| Junie | `effort` in `.junie/config.json` | any string |
| Kimi | `[thinking].effort` in the `config.toml` that `outputs.kimi.conf-file` names | any value |
| Qwen Code | `model.reasoningEffort` in `.qwen/settings.json` (0.28.0 or later) | `low`, `medium`, `high`, `xhigh`, `max`; each provider maps the value to its active model |
| Pi | `defaultThinkingLevel` in `.pi/settings.json` | `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` |

## Support by target {#support-by-target}

| Target | `permissions` | `model` |
|---|---|---|
| Claude Code, Qoder, Kilo Code, OpenCode | yes | yes |
| Factory | `Bash` rules only; `ask` goes to `commandDenylist` (which prompts), `deny` to `commandBlocklist` | yes |
| Codex | `Bash` rules via `outputs.codex.exec-policies-from-permissions` ([translation](@/docs/targets/codex-reference.md#translate-bash-permissions)); otherwise a note | yes |
| Windsurf | yes | no |
| Augment | `allow`, `ask`, and `deny`, for whole tools and `Bash` prefixes; path rules raise a note | `defaultModel`, read by Auggie 1.x |
| Cursor | `allow` and `deny` in `.cursor/cli.json`, for the Cursor CLI ([details](@/docs/targets/cursor-reference.md#permissions)) | no |
| Copilot, Junie, Gemini, Kiro | no; Copilot, Junie, and Kiro report the whole policy in a note | yes |

Other targets support neither field. Unsupported fields raise a sync note; output for other tools continues. A rule the vendor cannot scope, such as a path rule on Factory's shell lists, raises a note instead of widening.

A shared Claude model name raises the same note as an [agent `model`](@/docs/spec-format/agents.md#per-target-model-and-effort) on Codex, Gemini, OpenCode, Kilo Code, Factory, and Kiro; Codex skips it when `outputs.codex.config.model` or the [captured overlay](@/docs/targets/codex-reference.md#codex-config) sets the model. Import writes a tool's model only for that tool, as `model: {<tool>: <name>}`, because model names differ between vendors.

## Target-specific keys {#target-specific-keys}

Tool-specific keys go under `x-<target>` and merge into that tool's settings file (`x-factory.sandbox` reaches `.factory/settings.json`). When sync also writes the same key:

- Lists are joined, with the entries sync writes first.
- Objects merge key by key, at every level.
- A single value such as `model` is replaced.
- A list and a string cannot merge. The `x-<target>` value wins, with a sync note.
- Maps of whole records (`x-qoder.mcpServers`, `x-augment.mcpServers`) merge by name. A server both sides name comes whole from the `x-<target>` block.
- Blocks with a fixed order (`x-qoder.hooks`, `x-augment.hooks`) keep their order, with your own events added at the end.

Four keys take one type each: `x-augment.toolPermissions` a list, and `x-windsurf.permissions`, `x-kilo.permission`, and `x-opencode.permission` an object. Any other type is skipped with a sync note naming the key, and the rules from your `permissions` are still written.

Exceptions:

- Codex: `.codex/config.toml` comes from the captured overlay plus `outputs.codex.config`, so an `x-codex` block raises a sync note that names both.
- Ellipsis reads no portable field, only `x-ellipsis`: its reviewer, pull request filters, posting rules, and budget go to `.ellipsis/code-review.yaml`, and the environment, its repository, and native options to `.ellipsis/environments/<name>.yaml`. A later spec replaces each top-level block it sets ([details](@/docs/targets/ellipsis-reference.md#how-specs-map-to-the-file)).
- OpenCode V2 MCP imports use `x-opencode.mcp` to keep native connection details, disabled states, OAuth settings, and separate timeouts. Keep it in a settings spec, not in MCP specs. See [OpenCode](@/docs/targets/opencode.md).

## Session retention {#session-retention}

Keep this personal setting under `~/.agnostic-ai/settings/`, outside team project specs:

```yaml
# ~/.agnostic-ai/settings/retention.yaml
session-retention-days: 3650
```

`sync --scope user` writes the tool's own age limit; unset leaves it alone. Values that are not an integer of 1 or more fail validation (LINT045). Project sync only prints a note.

| Tool | User settings file | Native key |
|------|--------------------|------------|
| Claude Code | `~/.claude/settings.json` | `cleanupPeriodDays: 3650` |
| CodeBuddy Code | `~/.codebuddy/settings.json` | `cleanupPeriodDays: 3650` |
| Gemini CLI | `~/.gemini/settings.json` | `general.sessionRetention.maxAge: "3650d"` |
| Qoder CLI | `~/.qoder/settings.json` | `general.sessionRetention.maxAge: "3650d"` |

- An `x-claude.cleanupPeriodDays` or `x-codebuddy.cleanupPeriodDays` value wins for its target.
- An existing invalid Claude value, including `cleanupPeriodDays: 0`, can pause cleanup. Sync leaves it unchanged, even with `--backup`, and asks you to fix it by hand or set `x-claude.cleanupPeriodDays`.
- If Gemini or Qoder has `general.sessionRetention.enabled: false`, sync leaves its age limit alone and prints a note. It never sets the cleanup switch or `maxCount`, and Gemini's existing `maxCount` can still remove younger sessions.
- Other targets print a note. Kilo's deletion switch and Factory's organization setting are outside this field.
- Lowering the limit warns that older sessions will be deleted, including when removing the field restores a shorter native limit. Pass `--backup` to replace a native value set outside agnostic-ai.
- Removing the field restores the original value's bytes, even one that already matched, and removes an empty file that sync created. A value changed by hand stays yours. This restore data needs ownership state version 7 or later. User-level sync now writes version 10; older releases refuse that state rather than discard it.
- `explain --scope user settings/retention.yaml` lists the mapped keys.

agnostic-ai changes settings only; it never reads, copies, or archives transcripts. Claude Code also applies this cutoff to some other session data and orphaned worktrees, and Desktop and Cowork transcripts have a separate setting. See [Claude cleanup](https://code.claude.com/docs/en/claude-directory), [Gemini configuration](https://geminicli.com/docs/reference/configuration/), and [Qoder settings](https://docs.qoder.com/cli/settings-reference).

## Voice dictation {#voice-dictation}

Keep this personal setting under `~/.agnostic-ai/settings/`, outside team project specs:

```yaml
# ~/.agnostic-ai/settings/voice.yaml
voice:
  enabled: true
  mode: tap
  language: es
```

`sync --scope user` writes each member the tool has a key for; an unset member leaves the tool's value alone. `mode` is `hold` (hold a key while you talk) or `tap` (tap to start and stop). `language` is passed as written. Other values fail validation (LINT054). Project sync only prints a note.

| Tool | User settings file | Native keys |
|------|--------------------|-------------|
| Claude Code | `~/.claude/settings.json` | `voice.enabled`, `voice.mode` |
| Qwen Code | `~/.qwen/settings.json` | `general.voice.enabled`, `general.voice.mode`, `general.voice.language` |
| Gemini CLI | `~/.gemini/settings.json` | `experimental.voiceMode`, `experimental.voice.activationMode` (`hold` becomes `push-to-talk`, `tap` becomes `toggle`) |
| Grok Build | `~/.grok/config.toml` | `ui.voice_capture_mode` (`tap` becomes `toggle`), `ui.voice_stt_language` |
| Mistral Vibe | `~/.vibe/config.toml` | `voice_mode_enabled` |
| Kiro CLI | `~/.kiro/settings/cli.json` | Literal root key `voice.language` |
| Warp | [Platform settings file](@/docs/targets/warp-reference.md#voice-dictation) | `agents.voice.voice_input_enabled`, `agents.voice.voice_input_language` |

- Claude Code takes its dictation language from its response language, so sync does not write `voice.language` there and prints a note. Set `x-claude.language` to change both.
- Qwen keeps dictation off until a voice model is chosen. Run `/model --voice` or set `x-qwen.voiceModel`; sync prints a note when no model is configured.
- Gemini's voice keys and Vibe's voice mode are experimental. Gemini has no language key; Vibe has no mode or language key. Sync prints a note for each member without a native key.
- Grok has no dictation on/off key: `/voice` always works. Set `x-grok.ui.voice_keybind_enabled` to control its shortcut separately. `GROK_HOME` moves its user configuration directory.
- Kiro CLI maps language only; `auto` detects it. Its on/off and capture mode members print a note. `KIRO_HOME` moves the user directory. Native `x-kiro` keys use literal dotted names, such as `voice.language`.
- Other targets print a note naming their limit.
- An `x-<target>` key wins over the portable member it overlaps, including a parent map. Set keys such as `x-claude.voice.autoSubmit` there.
- Pass `--backup` to replace a native value set outside agnostic-ai.
- Removing the field restores each key's original bytes, removes an empty `voice` object that sync created, and removes an empty file that sync created. A value changed by hand stays yours.
- `explain --scope user settings/voice.yaml` lists the mapped keys.

`import --scope user` reads these keys into one shared `voice` map. Tool arguments select the native files to read, so importing Claude alone can supply voice settings to Qwen on a later sync. A tool whose home spec already sets `voice` is skipped. Import excludes those tools with `targets-exclude` to keep their existing values; model and effort values for excluded tools go into `settings/imported-models.yaml`.

An earlier model import can receive a missing `voice` field without changing its existing values or comments. Import uses `settings/imported-voice.yaml` when scope, formatting, or other incoming settings require a separate file. If that name is already in use by either layer, it selects a numbered name.

Values from the first tool named win when tools disagree; import prints the values it skipped. Claude's response language stays out of `voice.language`. Its older `voiceEnabled` key is read when the `voice` object is absent; sync reuses a matching legacy value without adding a nested key.

See [Claude voice dictation](https://code.claude.com/docs/en/voice-dictation), [Qwen settings](https://qwenlm.github.io/qwen-code-docs/en/users/configuration/settings/), [Gemini's voice schema](https://github.com/google-gemini/gemini-cli/blob/main/packages/cli/src/config/settingsSchema.ts), [Grok settings](https://docs.x.ai/build/settings/reference), and [Vibe voice mode](https://github.com/mistralai/mistral-vibe#voice-mode).
