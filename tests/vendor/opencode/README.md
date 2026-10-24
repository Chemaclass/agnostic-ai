# OpenCode hook compatibility

Run `make opencode-hooks-test`. The same check runs in `make ci-local` and CI. Normal Go tests do not need Node or Bun.

The check renders actual hook specs, compiles their modules against released TypeScript packages, and loads them with the released public `Host.load`. It calls the registered tool callbacks with types from that release and real child processes. It checks arguments, native JSON stdin, the triggering session's checkout, environment isolation, matching, sequential execution, blocking, failures, timeouts, and unload cleanup. The V1 control uses Bun's real `$` helper.

Pins: OpenCode V2 plugin 2.0.26, OpenCode V1 plugin 1.3.10, Bun 1.3.10. Bun is installed through its official optional platform packages with install scripts disabled. This is a test dependency only.

The V1 plugin's Zod dependency is overridden to 4.6.6 for a security fix. See [build dependency checks](../../../docs/internal/build-dependencies.md#check-npm-dependencies) for the advisory and removal condition.

On macOS and Linux, the suite proves real command execution, blocking, process-tree cleanup, and unload. On Windows, the released loader registers V2 callbacks, setup warns, matching callbacks refuse before session lookup or command execution, and unmatched events do not block. The actual V1 control runs on every OS. No Windows V2 launcher or startup measurement is included.

The V2 fixture uses the public loader, not a running OpenCode server. Core's extra check for a default ID/setup definition is private. Its contract is established from [the released loader](https://github.com/anomalyco/opencode/blob/5e73d5cb1f788637b1b3d5cf1298ea694cbad0c5/packages/core/src/plugin/module.ts#L77), and the loaded definition is checked before setup. No model or remote service is used.
