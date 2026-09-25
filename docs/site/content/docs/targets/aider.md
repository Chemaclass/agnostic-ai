+++
title = "Aider"
description = "How agnostic-ai emits Aider configuration: native paths, capability limits, and output options."
weight = 60

[extra]
group = "Reference"
target_id = "aider"
+++

# Aider (`aider`)

## Output

```
CONVENTIONS.md           # pointer body + inlined rules block (written by sync)
.aider.conf.yml          # only when conf-file is set
.aiderignore             # only when the project has ignore entries
```

- **Rules**: `CONVENTIONS.md` holds the pointer body plus a sentinel-marked `## Rules` block with unscoped rule bodies inline. `import aider` strips that block. Load it with `aider --read CONVENTIONS.md`.
- **Auto-load**: set `outputs.aider.conf-file: .aider.conf.yml` to merge a `read:` entry into Aider's [project config](https://aider.chat/docs/config/aider_conf.html). `model` and `weak-model` go into the same file when set. Existing keys are kept, and the `read:` list is de-duplicated.
- **Ignore**: ignore entries write `.aiderignore` in the project root, Aider's default path ([Aider config docs](https://aider.chat/docs/config/aider_conf.html)), so no wiring is needed. If you point `outputs.aider.ignore-file` elsewhere, Aider reads it only when the config file's `aiderignore:` key names that path.

## Config keys

| Key | Default | Notes |
|---|---|---|
| `outputs.aider.conf-file` | empty, opt-in | |
| `outputs.aider.model` | | |
| `outputs.aider.weak-model` | | |
| `outputs.aider.rules-file` | unset | writes a legacy merged document and skips the pointer-body write |
| `outputs.aider.ignore-file` | `.aiderignore` | |

## Verify

1. Install: `python -m pip install -U aider-chat` (or `pipx install aider-chat`).
2. Check the tree: `ls CONVENTIONS.md .aider.conf.yml`, and `head -1 .aider.conf.yml` shows the provenance header.
3. Validate YAML: `python -c "import yaml,sys; yaml.safe_load(open('.aider.conf.yml'))"`.
4. `aider --config .aider.conf.yml --no-stream --message "list the rules you were told to follow"`. The banner lists the `read:` paths, including `CONVENTIONS.md`, and the reply reflects the rules.
5. No `Warning:` lines mentioning `CONVENTIONS.md` or `.aider.conf.yml`.
