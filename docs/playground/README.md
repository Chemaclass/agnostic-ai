# Playground

[All docs](../README.md) · [Contributor setup](../../CONTRIBUTING.md)

In-browser playground for agnostic-ai. Paste a spec, pick targets, see
each adapter's emission live. Runs entirely client-side via WebAssembly,
so the page costs zero server resources and works on any static host.

## Try it

Open the [published playground](https://chemaclass.github.io/agnostic-ai/playground/). The [Pages workflow](../../.github/workflows/playground.yml) rebuilds it on pushes to `main` and manual dispatch.

## Run locally

```bash
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

Adapters use an emission session in capture mode, recording output in memory for the browser.

## Build size

Measure the artifact after building:

```bash
ls -lh docs/playground/agnostic-ai.wasm
gzip -c docs/playground/agnostic-ai.wasm | wc -c
```

The first command shows raw size; the second shows gzip size in bytes. Transfer size depends on your static host's compression settings.
