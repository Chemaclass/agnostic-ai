# Playground

[All docs](../README.md) · [Contributor setup](../../CONTRIBUTING.md)

Edit a spec and see the configuration for five coding tools as you type. Tabs are disabled when a tool does not support the selected spec kind. Follow the link to see other supported tools.

The playground runs Go code as WebAssembly in your browser. A static web host can serve it.

## Try it

Open the [published playground](https://agnostic-ai.org/playground/). The [Pages workflow](../../.github/workflows/playground.yml) rebuilds it on pushes to `main` and manual dispatch.

## Run locally

```bash
make playground-serve       # builds the site and opens /playground/ on port 8080
```

Open http://127.0.0.1:8080/playground/ to load the playground with the Zola site's navigation. Serve it over HTTP because browsers refuse to fetch `.wasm` from a `file://` page.

## What's in this directory

| File | Purpose |
|------|---------|
| `index.html` | Two panels: spec input on the left, generated files on the right. |
| `style.css` | Layout + dark/light theme. |
| `playground.js` | Connects seven spec kinds and five demo targets, links to the rest, disables tabs for unsupported kinds, and waits briefly after typing before generating output. |
| `wasm_exec.js` | JavaScript helper for Go's browser code. Generated and ignored by Git. |
| `agnostic-ai.wasm` | Built from `cmd/agnostic-ai-wasm`. Generated and ignored by Git. |

## How it works

`cmd/agnostic-ai-wasm/main.go` makes three functions available to JavaScript:

- `agnosticAIRender(kind, body, targets)` returns
  `{ files: [{target, path, content}], errors: [{target, message}] }`.
- `agnosticAITargets()` returns the list of every adapter included in
  the executable, so the page can build the target picker from that list.
- `agnosticAICapabilities()` returns each target's supported spec kinds
  from its adapter declaration. The Pages build picks up
  `caps.Supports` changes without a separate playground data edit.

Each adapter keeps its generated files in memory for the browser.

## Build size

Measure the generated file after building:

```bash
ls -lh docs/playground/agnostic-ai.wasm
gzip -c docs/playground/agnostic-ai.wasm | wc -c
```

The first command shows the raw size. The second shows the gzip size in bytes. Transfer size depends on the host's compression settings.
