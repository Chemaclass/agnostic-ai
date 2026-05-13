# agnostic-ai for JetBrains (planned)

> Status: **scaffolding placeholder**. The functional plugin lives in
> [issue #44](https://github.com/Chemaclass/agnostic-ai/issues/44) and
> is shipped after the VS Code extension stabilizes.

## Why a separate directory now

Both editor extensions share a goal — give authors live feedback on
agnostic-ai specs without leaving the IDE — but their build chains do
not overlap. Co-locating the plugin source under `editors/jetbrains/`
keeps the project layout obvious to contributors:

```
editors/
├── vscode/        # TypeScript, ships first
└── jetbrains/     # Kotlin + IntelliJ Platform Gradle Plugin
```

When the JetBrains plugin lands it will mirror the VS Code surface:

- Schema validation and completion for `agnostic.config.yaml` (covered
  natively by the IntelliJ YAML support once we contribute the schema).
- Run actions in the Tools menu for `sync`, `sync --check`,
  `doctor --fix`, `status`.
- A line marker (the JetBrains analogue of VS Code's codelens) above
  each spec for "Render to <target>".
- A status bar widget showing the current drift count.

## Why not now

Cutting a Marketplace-ready JetBrains plugin requires Gradle, Kotlin
toolchain, and a JetBrains Marketplace account that is separate from
the VS Code Marketplace publisher. Doing both in one PR would buffer
the higher-leverage VS Code work behind a longer review cycle.

## Want to help

Pick up [issue #44](https://github.com/Chemaclass/agnostic-ai/issues/44)
and open a PR. Aim for the IntelliJ Platform Gradle Plugin (current
recommended stack) and the same "shell out to the user's installed
binary; ship no bundled binary" rule that the VS Code extension
follows.
