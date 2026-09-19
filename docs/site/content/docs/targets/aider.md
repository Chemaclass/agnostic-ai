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

`CONVENTIONS.md` carries the pointer body plus a sentinel-marked `## Rules` block with unscoped rule bodies inline, so the conventions reach Aider by default; `import aider` strips that block. Wire the file in via `aider --read CONVENTIONS.md`. Set `outputs.aider.conf-file: .aider.conf.yml` to also merge a `read:` entry into Aider's [project config](https://aider.chat/docs/config/aider_conf.html) so the file auto-loads. `model` and `weak-model` propagate into the same file when set. Pre-existing keys are preserved; the `read:` list de-duplicates.

Ignore entries write `.aiderignore` in the project root. That is Aider's own default, so it needs no wiring: "Specify the aider ignore file (default: .aiderignore in git root)" ([aider_conf.html](https://aider.chat/docs/config/aider_conf.html)). Point `outputs.aider.ignore-file` somewhere else and Aider only reads it if the config file's `aiderignore:` key names that path.

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
2. Check the tree: `ls CONVENTIONS.md .aider.conf.yml`, `head -1 .aider.conf.yml` must start with the provenance header.
3. Validate YAML: `python -c "import yaml,sys; yaml.safe_load(open('.aider.conf.yml'))"`.
4. `aider --config .aider.conf.yml --no-stream --message "list the rules you were told to follow"`. The banner prints the resolved `read:` paths (including `CONVENTIONS.md`); the response reflects the rules text.
5. No `Warning:` lines mentioning `CONVENTIONS.md` or `.aider.conf.yml`.
