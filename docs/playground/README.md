# Playground

In-browser playground for agnostic-ai. Paste a spec, pick targets, see
each adapter's emission live. Runs entirely client-side via WebAssembly,
so the page costs zero server resources and works on any static host.

## Try it

The published page lives at GitHub Pages once the `Playground` workflow
runs (manual dispatch or release tag). The URL appears in the run
summary.

## Run locally

```bash
make playground-build       # compiles the WASM and copies wasm_exec.js
make playground-serve       # builds, then serves on http://127.0.0.1:8080
```

`file://` protocol does **not** work. Browsers refuse to fetch the
`.wasm` from a `file://` page. Use the `playground-serve` target or any
static HTTP server pointed at `docs/playground/`.

## What's in this directory

| File | Purpose |
|------|---------|
| `index.html` | Two-pane UI: spec input on the left, emitted outputs on the right. |
| `style.css` | Layout + dark/light theme. |
| `playground.js` | Wires up the textarea, target checkboxes, tabs; debounces input. |
| `wasm_exec.js` | Go toolchain shim. Generated; gitignored. |
| `agnostic-ai.wasm` | Built from `cmd/agnostic-ai-wasm`. Generated; gitignored. |

## How it works

`cmd/agnostic-ai-wasm/main.go` exposes two globals to JavaScript:

- `agnosticAIRender(kind, body, targets)` returns
  `{ files: [{target, path, content}], errors: [{target, message}] }`.
- `agnosticAITargets()` returns the list of every adapter linked into
  the binary, so the UI can build the target picker dynamically.

Adapters run under capture mode (`adapters.StartCapture()`), so no
filesystem operations occur, perfect for the WASM sandbox.

## Build size

The current Go-toolchain WASM build clocks in around 5 MB raw, ~1.4 MB
gzipped. GitHub Pages serves with `Content-Encoding: gzip` automatically,
so visitors see the gzipped size on the wire.
