# Local configuration UI

Inspect project or user configuration in a local browser. Installed binaries include the client: run `agnostic-ai serve --open` from a configured project, or add `--global` for user configuration.

| Section | Shows |
| --- | --- |
| Context | The instructions file, rules, agents, and skills |
| Authority | The permission files, each rule as a chip |
| Tools | MCP servers, each with the tools `agnostic-ai mcp tools` last listed |
| Lifecycle | Every point in a session a hook can run at, with the coding tools that run it and the hooks configured there |
| Settings | Every built-in with its on/off state, the config file's values, and other settings specs |

Findings include a **Copy fix prompt** action. **Details** shows the suggested fix, explanation, and the exact prompt the button copies. Native settings group keys by tool. Target lists use logos with full names on hover.

Source lists use aligned rows. Unconfigured fields stay in a disclosure, empty lifecycle stages show their descriptions on demand, and support groups stack on mobile. The findings summary jumps to the full list; errors remain above the controls.

Values render by type: a list as chips, a boolean as a switch, anything
else as text. Built-in switches share the same row layout, with previews and errors below. The project Caveman switch previews, applies, and undoes changes across the configured targets. Other built-in and boolean switches save their source files after validation. See [web changes and undo](../docs/site/content/docs/cli-reference/inspect.md#serve) for prerequisites and recovery. Lifecycle shows each tool that
runs a stage as its icon (`src/assets/tools/`, see its README). A Markdown or YAML item has an edit icon that opens
it in Cursor (`cursor://file/<absolute path>`). Each item carries the lint and doctor findings that name
its file. The JSON overview also reports whether generated files match
what `sync` would write.

`src/contract.ts` mirrors the Go types in `internal/overview/types.go`
and `internal/authority/types.go`. Update them together. `src/builtin-change.ts` validates the change responses from `internal/cli/serve_builtin_changes.go`.

## Run it against the server

From the repository root, serve a project or the user-level configuration:

```sh
go run ./cmd/agnostic-ai serve --api-only --dir internal/authority/testdata/demo-project --allow-origin http://127.0.0.1:5173
# Or serve user configuration with the same development origin:
go run ./cmd/agnostic-ai serve --api-only --global --allow-origin http://127.0.0.1:5173
```

Then, in `ui/`:

```sh
npm ci
npm run dev -- --host 127.0.0.1 --port 5173 --strictPort
```

Open `http://127.0.0.1:5173`. Vite forwards `/api` to
`http://127.0.0.1:4310`, preserving the original Host header. Set
`AGNOSTIC_AI_API_URL` to use another backend address. Other browser
origins need an exact `--allow-origin` on the Go server.

The bundled client runs on the same port as the API:

```sh
go run ./cmd/agnostic-ai serve --dir internal/authority/testdata/demo-project --open
```

For an unbundled development build, run `npm run build` in `ui/` and pass `--ui ui/dist`. `--api-only` leaves the browser interface off.

## Update the bundled client

Use Node.js 22 from the repository root:

```sh
make ui-build
make build
```

Commit `ui/embedded/` with changes to its inputs. The bundle records source and asset hashes; `make build` and Go tests reject stale files. CI and release preparation also rebuild from `package-lock.json` and compare every asset with the committed bundle. Plain Go builds and `go install` use the tracked bundle without Node.js.

`npm run bundle` writes the embedded client with root-relative asset URLs and no default snapshot source. It ignores `.env` files and `VITE_DEFAULT_SOURCE`, so a developer's static-preview settings cannot enter a release. `npm run build` still produces the relative-path standalone client in `dist/`. The embedded `third-party-notices.txt` contains the client dependency and icon licenses.

## Run it without a server

**Mock data.** Add `?mock=1` to the URL. The client reads
`src/fixtures/overview.mock.json`, the server's output for the demo
project, with the checkout's absolute path replaced so no machine path is committed. Regenerate it after a contract change:

```sh
go run ./cmd/agnostic-ai view overview --dir internal/authority/testdata/demo-project --json \
  | sed "s|$PWD/|/path/to/agnostic-ai/|g" > ui/src/fixtures/overview.mock.json
```

**Static snapshot.** Add `?source=<directory or URL>`, for example
`?source=./snapshot`. The client reads `<source>/overview.json` with
plain `fetch`. A relative source resolves against the page; an absolute
one must be http(s). Capture one from a running server:

```sh
mkdir -p snapshot
curl -s http://127.0.0.1:4310/api/views/overview > snapshot/overview.json
```

For a host with no server at all, such as GitHub Pages, build with a
default source so visitors need no query string:

```sh
VITE_DEFAULT_SOURCE=./snapshot npm run build
```

The build uses relative asset paths, so `dist/` runs from any directory.

## URL state

| Parameter | Meaning |
| --------- | ------- |
| `view`    | `context` (default), `authority`, `tools`, `lifecycle`, `settings` |
| `mock`    | `1` to read the bundled fixture |
| `source`  | A static snapshot directory or URL |

`Esc` closes the JSON inspector.

## Scripts

| Script              | What it does |
| ------------------- | ------------ |
| `npm run dev`       | Vite dev server with `/api` forwarded to 4310 |
| `npm run build`     | Standalone production build into `dist/` |
| `npm run bundle` | Regenerate tracked `embedded/` assets and their manifest |
| `npm run bundle:check` | Rebuild in a temporary directory and reject stale bundled assets |
| `npm run preview`   | Serves `dist/`, also forwarding `/api` |
| `npm run typecheck` | `tsc --noEmit` |
| `npm run test`      | Model tests with Vitest |
| `npm run e2e`       | End-to-end tests with Playwright |
| `npm run e2e:live`  | Real Go server, temporary sources, and browser edits |

## Tests

**Model tests** (`src/model/*.test.ts`) cover URL state, data source
parsing, and the fixture's conformance to the contract. Point them at
real server output with `OVERVIEW_FIXTURE` (a path relative to `ui/`).

**Fixture browser tests** exercise rendering, themes, keyboard controls,
and accepted/refused edit responses. The edit responses are mocked.

**Live browser tests** (`e2e/live.spec.ts`) start the built Go server
against temporary project and user sources. They load the embedded client without `--ui`, use real API requests,
reload persisted edits, enforce browser boundaries, list tools from a
local fake MCP server, and exercise the Vite proxy. They do not read or
change the user's configuration.

| Variable             | Meaning |
| -------------------- | ------- |
| `E2E_BASE_URL`       | The client to test; default `http://127.0.0.1:4310`, the Go server |
| `E2E_MOCK=1`         | Append `?mock=1` so the client reads the bundled fixture |
| `E2E_PREVIEW=1`      | Build and start `vite preview` on 4173 before the tests |
| `E2E_SCREENSHOT_DIR` | Where screenshots go; default `ui/test-results/screenshots/`, which git ignores |
| `E2E_CHROMIUM_PATH`  | A Chromium binary, only if the default launch cannot find one |
| `AGNOSTIC_E2E_BIN` | Built CLI for live tests; default is the repository's `agnostic-ai` executable |

```sh
# Against the bundled fixture, with Playwright starting the preview server
E2E_PREVIEW=1 E2E_MOCK=1 npm run e2e

# From the repository root, build the real server and client:
make ui-build
make build
(cd ui && npm run e2e:live)
```

Use `E2E_CHROMIUM_PATH` for an existing Chrome installation, or install
the browser matching the pinned Playwright version with
`npx playwright install chromium` from `ui/`. CI installs the pinned browser before running both suites.

The live suite also checks Caveman preview cancellation, apply for Claude and Codex, undo after reload, and conflicts that preserve later source and output edits.

## Source layout

- `src/contract.ts` the JSON contract, mirrored from Go
- `src/api.ts` loading from the server, the mock fixture, or a static snapshot
- `src/model/` URL state and normalization
- `src/components/` the shell, the section page, and the JSON inspector
- `src/styles.css` every colour as a token on `:root`, with a light theme
  under `[data-theme="light"]` and reduced-motion rules
