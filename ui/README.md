# Agnostic AI harness UI

The web client for the harness overview: one page per section of an
Agnostic AI configuration.

| Section | Shows |
| --- | --- |
| Context | The instructions file, rules, agents, and skills |
| Authority | The permission files, each rule as a chip |
| Tools | MCP servers, each with the tools `agnostic-ai mcp tools` last listed |
| Lifecycle | Every point in a session a hook can run at, with the coding tools that run it and the hooks configured there |
| Settings | Every built-in with its on/off state, the config file's values, and other settings specs |

Values render by type: a list as chips, a boolean as a switch, anything
else as text. A Markdown item has an **Open in Cursor** link
(`cursor://file/<absolute path>`). Each item carries the lint and doctor findings that name
its file, and the header shows whether each coding tool's generated files
are in sync with what `sync` would write.

The server owns the truth; the client owns presentation. `src/contract.ts`
mirrors the Go types in `internal/overview/types.go` (and, for the API it
still serves, `internal/authority/types.go`); change it only together
with them.

## Run it against the server

From the repository root, serve a project or the user-level configuration:

```sh
go run ./cmd/agnostic-ai serve --dir internal/authority/testdata/demo-project
go run ./cmd/agnostic-ai serve --global
```

Then, in `ui/`:

```sh
npm install
npm run dev
```

Open the address Vite prints (http://127.0.0.1:5173 by default). The dev
server and `npm run preview` both forward `/api` to the Go server on
`http://127.0.0.1:4310`.

The server can also host the built client itself, on the same port as
the API:

```sh
(cd ui && npm run build)
go run ./cmd/agnostic-ai serve --dir internal/authority/testdata/demo-project --ui ui/dist
```

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
| `npm run build`     | Production build into `dist/` |
| `npm run preview`   | Serves `dist/`, also forwarding `/api` |
| `npm run typecheck` | `tsc --noEmit` |
| `npm run test`      | Model tests with Vitest |
| `npm run e2e`       | End-to-end tests with Playwright |

## Tests

**Model tests** (`src/model/*.test.ts`) cover URL state, data source
parsing, and the fixture's conformance to the contract. Point them at
real server output with `OVERVIEW_FIXTURE` (a path relative to `ui/`).

**End-to-end tests** (`e2e/*.spec.ts`) drive the built client:

| Variable             | Meaning |
| -------------------- | ------- |
| `E2E_BASE_URL`       | The client to test; default `http://127.0.0.1:4310`, the Go server |
| `E2E_MOCK=1`         | Append `?mock=1` so the client reads the bundled fixture |
| `E2E_PREVIEW=1`      | Build and start `vite preview` on 4173 before the tests |
| `E2E_SCREENSHOT_DIR` | Where screenshots go; default `docs/internal/assets/metaprogramming/authority/` |
| `E2E_CHROMIUM_PATH`  | A Chromium binary, only if the default launch cannot find one |

```sh
# Against the bundled fixture, with Playwright starting the preview server
E2E_PREVIEW=1 E2E_MOCK=1 npm run e2e

# Against the real server, started with --ui ui/dist
E2E_BASE_URL=http://127.0.0.1:4310 npm run e2e
```

`@playwright/test` is pinned to 1.56.1 to match the preinstalled
Chromium build; do not run `playwright install`.

## Source layout

- `src/contract.ts` the JSON contract, mirrored from Go
- `src/api.ts` loading from the server, the mock fixture, or a static snapshot
- `src/model/` URL state and normalization
- `src/components/` the shell, the section page, and the JSON inspector
- `src/styles.css` every colour as a token on `:root`, with a light theme
  under `[data-theme="light"]` and reduced-motion rules
