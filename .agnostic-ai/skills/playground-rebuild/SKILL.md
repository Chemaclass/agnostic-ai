---
name: playground-rebuild
description: Rebuild and serve the WASM playground at docs/playground/. Use after editing the playground source or any code reachable from cmd/agnostic-ai-wasm.
---

# playground-rebuild

The hosted playground at https://agnostic-ai.org/playground/ runs the same code as the CLI, compiled to WebAssembly via `cmd/agnostic-ai-wasm`.

## Build

```
make playground-build
```

Writes `docs/playground/agnostic-ai.wasm` and copies `wasm_exec.js`, the helper script from the Go toolchain. Both are gitignored; CI regenerates them on push to `main`.

## Serve locally

```
make playground-serve
```

Builds the Zola site and WebAssembly bundle, assembles the same files that GitHub Pages publishes, then serves `_site/` with `python3 -m http.server 8080`. Open http://127.0.0.1:8080/playground/.

`file://` does not work because browsers refuse `WebAssembly.instantiateStreaming` on local files.

## After edits

For frontend or Go source changes, stop the server started for this task and rerun `make playground-serve`, then refresh the browser. This rebuilds and copies the bundle into `_site/playground/`. `make playground-build` alone updates `docs/playground/`, leaving the served copy stale.

Exercise the changed spec and target in the browser and inspect its generated output or error. Record the scenario and result. Select any additional checks from repository-root `docs/internal/contributing.md`, section "Choose checks for your change".

## Clean

```
make playground-clean
```

Removes the generated `.wasm` and `wasm_exec.js`.
