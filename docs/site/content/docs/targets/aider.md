+++
title = "Aider"
description = "What agnostic-ai writes for Aider: file paths, what it supports, and output settings."
weight = 60

[extra]
group = "Reference"
target_id = "aider"
+++

# Aider (`aider`)

[Aider](https://aider.chat/docs/config/aider_conf.html) reads rules from `CONVENTIONS.md`. agnostic-ai writes that file, an optional `.aider.conf.yml` that loads it, and `.aiderignore`.

## Output

```
CONVENTIONS.md           # pointer body + inlined rules block (written by sync)
.aider.conf.yml          # only when conf-file is set
.aiderignore             # only when the project has ignore entries
```

- **Rules**: `CONVENTIONS.md` holds the pointer body plus a marked `## Rules` block with the unscoped rule bodies. `import aider` strips that block. Load it with `aider --read CONVENTIONS.md`.
- **Auto-load**: set `outputs.aider.conf-file: .aider.conf.yml` to add a `read:` entry to Aider's [project config](https://aider.chat/docs/config/aider_conf.html). `model` and `weak-model` go into the same file when set. Your other keys stay, and the `read:` list has no duplicates. Each sync rewrites the file with sorted keys and drops its comments.
- **Ignore**: ignore entries go to `.aiderignore` in the project root. That is Aider's default path ([Aider config docs](https://aider.chat/docs/config/aider_conf.html)), so it needs no setup. If you point `outputs.aider.ignore-file` elsewhere, Aider reads it only when the config file's `aiderignore:` key names that path.

{% <details summary="What sync removes from .aider.conf.yml"> %}
When aider leaves `targets` or `conf-file` is unset, sync removes only `model`, `weak-model`, and the `read:` entry it added. It keeps your keys in their order, with whatever comments they still have, and drops the header. When `rules-file` changes, sync removes the `read:` entry it added for the old path.
{% </details> %}

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.aider.conf-file` | empty, opt-in | |
| `outputs.aider.model` | | |
| `outputs.aider.weak-model` | | |
| `outputs.aider.rules-file` | unset | writes all rules into one legacy file and skips the pointer-body write |
| `outputs.aider.ignore-file` | `.aiderignore` | |

With [`builtins: [handoff]`](@/docs/handoff.md), a configured `rules-file` includes the handoff instructions.

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install: `python -m pip install -U aider-chat` (or `pipx install aider-chat`).
2. Check the files: `ls CONVENTIONS.md .aider.conf.yml`, and `head -1 .aider.conf.yml` shows the generated-file header.
3. Validate YAML: `python -c "import yaml,sys; yaml.safe_load(open('.aider.conf.yml'))"`.
4. Run `aider --config .aider.conf.yml --no-stream --message "list the rules you were told to follow"`. The banner lists the `read:` paths, including `CONVENTIONS.md`, and the reply reflects the rules.
5. Check that no `Warning:` line mentions `CONVENTIONS.md` or `.aider.conf.yml`.
