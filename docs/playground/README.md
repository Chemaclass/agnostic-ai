# Playground

[All docs](../README.md) · [Contributor setup](../../CONTRIBUTING.md)

In-browser playground for agnostic-ai. Edit a spec and see what five
well-known targets receive as you type. A target that does not support the
spec kind shows as a disabled tab, and a link leads to the rest.
Runs entirely in your browser using WebAssembly, which lets the browser run the compiled Go code. No server computes the output, so any static website host can serve the page.

## Try it

Open the [published playground](https://agnostic-ai.org/playground/). The [Pages workflow](../../.github/workflows/playground.yml) rebuilds it on pushes to `main` and manual dispatch.

## Run locally

```bash
make playground-serve       # builds the site and opens /playground/ on port 8080
```

`file://` protocol does **not** work. Browsers refuse to fetch the
`.wasm` from a `file://` page. The playground also shares navigation assets
with the Zola site, so use the assembled page at
http://127.0.0.1:8080/playground/.

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
