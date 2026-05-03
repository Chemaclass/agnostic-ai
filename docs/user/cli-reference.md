# CLI reference

```
agnostic-ai [command] [flags]
```

## init

Scaffold a project. Creates `agnostic.config.yaml` and empty `agents/`, `skills/`, `rules/`, `hooks/`. Errors if `agnostic.config.yaml` exists.

```bash
agnostic-ai init
```

## validate

Load all specs, report parse errors. Writes nothing.

```bash
agnostic-ai validate
```

```
loaded 12 entries. ok.
```

## list

Print all loaded specs as `kind<tab>name`.

```bash
agnostic-ai list
```

## sync

Emit per-target configs.

```bash
agnostic-ai sync [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--dry-run` | Print to stdout instead of writing files |

```bash
agnostic-ai sync                    # all targets in config
agnostic-ai sync -t claude          # only claude
agnostic-ai sync -t claude,cursor   # subset
agnostic-ai sync --dry-run          # preview
```

## version

```bash
agnostic-ai --version
```
