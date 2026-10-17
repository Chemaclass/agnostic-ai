# Agnostic AI harness UI

The web client for the harness views: the **feed** of evidence-backed
decisions, an overview page for each harness section (Context, Memory,
Control, Tools, Verification), and the **Authority** deep view, which
answers which exact resources an agent can reach on a given coding tool,
and why.

The server owns the truth: names, effects, precedence, provenance, and
impact analysis. The client owns presentation: selection, layout,
interaction, and motion. `src/contract.ts` mirrors the Go types in
`internal/authority/types.go` and `internal/overview/types.go`; change it
only together with them.

## Run it against the server

From the repository root, start the server on the demo project:

```sh
go run ./cmd/agnostic-ai serve --dir internal/authority/testdata/demo-project
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

**Mock data.** Add `?mock=1` to the URL. The client reads the handwritten
fixtures in `src/fixtures/authority.mock.json` and
`src/fixtures/overview.mock.json` (the demo project, the coding agent on
Claude Code, before the first sync). "Validate on server" still posts to
`/api/views/authority/impact`, so it reports that the server is
unreachable unless one is running; the end-to-end tests stub that request
inside the page.

**Static snapshot.** Add `?source=<directory or URL>`, for example
`?source=./snapshot`. The client reads, with plain `fetch`:

- `<source>/authority.<agentId>.<target>.json` for the selected agent and
  coding tool, when that file exists, otherwise `<source>/authority.json`
- `<source>/overview.json`

A relative source resolves against the page; an absolute one must be
http(s). The header shows a **Static snapshot · revision …** chip. A view
the snapshot does not hold falls back to `authority.json` with a notice
naming what is shown instead. The proposal panel still shows the client
preview, but "Validate on server" is disabled with the note *needs a
running server*. Dismissals and snoozes keep working in this browser.

To capture a snapshot from a running server:

```sh
mkdir -p snapshot
curl -s http://127.0.0.1:4310/api/views/overview > snapshot/overview.json
curl -s http://127.0.0.1:4310/api/views/authority > snapshot/authority.json
curl -s 'http://127.0.0.1:4310/api/views/authority?agentId=reviewer&target=codex' \
  > snapshot/authority.reviewer.codex.json
```

For a host with no server at all, such as GitHub Pages, build with a
default source so visitors need no query string:

```sh
VITE_DEFAULT_SOURCE=./snapshot npm run build
```

The build uses relative asset paths, so `dist/` runs from any directory.
Copy the snapshot directory next to `index.html`.

`?mock=1` is not implemented as an alias for `?source=/src/fixtures`:
the built files and `vite preview` do not serve `/src`, and mock mode
must keep posting the impact request so the end-to-end tests can stub
it, while a static snapshot disables validation.

## URL state

Every navigable choice is in the query string, so a link reproduces the
screen.

| Parameter  | Meaning |
| ---------- | ------- |
| `view`     | `feed` (default), `context`, `memory`, `control`, `authority`, `tools`, `verification`, `inspect` |
| `target`   | The coding tool to resolve effects for (from the view's `harness.available`) |
| `agentId`  | The subject (from the view's `agents`) |
| `domain`   | Authority: the selected capability domain |
| `resource` | Authority: the selected resource |
| `tab`      | Authority domain page: `resources` (default) or `circuit` |
| `card`     | Feed: the card to highlight |
| `proposal` | Authority: a proposal draft as JSON (`{ agentId, target, revision, changes }`) |
| `mock`     | `1` to read the bundled fixtures |
| `source`   | A static snapshot directory or URL |

## Keyboard

On the feed: `j` and `k` move the highlight, `y` takes the highlighted
card's first action, `n` dismisses it. On a touch screen, swipe the card
right to accept or left to dismiss. `Esc` closes the JSON inspector and
the proposal menu.

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

**Model tests** (`src/model/*.test.ts`) cover the pure code: grouping
resources by domain and provider, the resource status pill, effect
wording, circuit layout determinism, URL state and data source parsing,
the client proposal preview, and feed ranking and dismissal. They read
the mock fixtures by default; point them at real server output with
`AUTHORITY_FIXTURE` and `OVERVIEW_FIXTURE` (paths relative to `ui/`):

```sh
AUTHORITY_FIXTURE=src/fixtures/authority.demo.json npm run test
```

Assertions that only hold for the mock's exact names are skipped then;
`fixture.test.ts` checks the file against the contract.

**End-to-end tests** (`e2e/*.spec.ts`) drive the built client:

| Variable             | Meaning |
| -------------------- | ------- |
| `E2E_BASE_URL`       | The client to test; default `http://127.0.0.1:4310`, the Go server |
| `E2E_MOCK=1`         | Append `?mock=1`; the impact request is stubbed inside the page |
| `E2E_PREVIEW=1`      | Build and start `vite preview` on 4173 before the tests |
| `E2E_SCREENSHOT_DIR` | Where screenshots go; default `docs/internal/assets/metaprogramming/authority/` |
| `E2E_CHROMIUM_PATH`  | A Chromium binary, only if the default launch cannot find one |

```sh
# Against the bundled fixtures
npm run build
npx vite preview --host 127.0.0.1 --port 4173 &
E2E_MOCK=1 E2E_BASE_URL=http://127.0.0.1:4173 npm run e2e

# The same, with Playwright starting the preview server
E2E_PREVIEW=1 E2E_MOCK=1 npm run e2e

# Against the real server, started with --ui ui/dist so it hosts the
# built client and /api together
E2E_BASE_URL=http://127.0.0.1:4310 npm run e2e
```

`@playwright/test` is pinned to 1.56.1 to match the preinstalled
Chromium build; do not run `playwright install`. The tests save
screenshots at 1280×900 (and 390×844 for the phone feed) to the
screenshot directory.

## Source layout

- `src/contract.ts` the JSON contract, mirrored from Go
- `src/api.ts` loading from the server, the mock fixtures, or a static snapshot
- `src/model/` pure, tested logic: wording, grouping, status, circuit
  layout, URL state, proposals, feed, diff, normalization
- `src/components/` the screens
- `src/styles.css` every colour as a token on `:root`, with a light theme
  under `[data-theme="light"]` and reduced-motion rules
