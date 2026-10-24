# Playground

[All docs](../README.md) · [Contributor setup](../../CONTRIBUTING.md)

Edit a spec and see the configuration for five coding tools as you type. Tabs are disabled when a tool does not support the selected spec kind. Follow the link to see other supported tools.

The playground runs the compiled Go code in your browser through WebAssembly. Any static website host can serve it; no server computes the output.

## Try it

Open the [published playground](https://agnostic-ai.org/playground/). The [Pages workflow](../../.github/workflows/playground.yml) rebuilds it on pushes to `main` and manual dispatch.

## Run locally

```bash
make playground-serve       # builds the site and opens /playground/ on port 8080
```

Open http://127.0.0.1:8080/playground/. This serves the playground with the Zola site's navigation assets. Opening the HTML file directly will not work: browsers refuse to fetch `.wasm` from a `file://` page.

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
  from its adapter declaration. The Pages build therefore picks up a
  `caps.Supports` change without a separate playground data edit.

The code that writes each tool's configuration records output in memory for the browser instead of writing files.

## Build size

Measure the generated file after building:

```bash
ls -lh docs/playground/agnostic-ai.wasm
gzip -c docs/playground/agnostic-ai.wasm | wc -c
```

The first command shows raw size; the second shows gzip size in bytes. Transfer size depends on your static host's compression settings.
