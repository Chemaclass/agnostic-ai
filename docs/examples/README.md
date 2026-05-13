# Examples

The fastest way to see real specs is `agnostic-ai init --demo`. It
seeds one minimal example per source folder so you can run
`agnostic-ai sync --dry-run` immediately and see what every adapter
produces.

```bash
agnostic-ai init --demo
agnostic-ai sync --dry-run
```

The seeded files live in your project after init; edit or delete to
taste. The canonical copies ship with the CLI binary under
`internal/cli/initdata/`.

## Reference config

[agnostic-ai.yaml](agnostic-ai.yaml) shows every available
knob with its default value. Drop into your project root and trim to
what you need; every section is optional.

See [user docs](../user/README.md) for the full spec format and
per-target behavior.
