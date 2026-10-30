# Local configuration UI

Inspect project or user configuration in a local browser. Installed binaries include the client: run `agnostic-ai view` from a project or any folder inside it. Outside a project it opens user configuration; `--global` selects that configuration explicitly.

| Section | Shows |
| --- | --- |
| Context | The instructions file, rules, agents, and skills |
| Authority | Permission rules, with shared or personal list editing and file previews |
| Tools | MCP servers, each with the tools `agnostic-ai mcp tools` last listed |
| Lifecycle | Every session stage and its hook counts; expand a stage for its tools and hooks, or use Expand all and Collapse all. With no hooks, a plain list of stages and their tools |
| Settings | Every built-in with its on/off state, the config file's values, and other settings specs |

Findings use compact rows with severity, check code, and source path. Long messages expand with **Show full message**; **Copy fix prompt** always copies the complete diagnostic. **Details** shows the suggested fix, explanation, and the exact prompt the button copies. With a running server, **Recheck** reloads the overview through the shared refresh and compares finding ids, which ignore line numbers, to say whether that finding still appears. A failed refresh, or the `lint-incomplete` finding the server sends when lint stopped early, is never read as a fix. Doctor findings that disappear are shown as not confirmed, because the overview's doctor checks skip files they cannot read. Native settings group keys by tool. Target lists use logos with full names on hover.

**Context size**, at the end of Context, reads `GET /api/views/context?target=<tool>&file=<path>` from a running server: the `explain --context` report for one target, with scoped rules counted for the file in **Working on file** when the tool supports one, with duplicated startup passages, edit links, and **Copy review prompt**. **Refresh** fetches it again and labels the previous counts **Before refresh**. Mock mode reads `src/fixtures/context.mock.json`; snapshots show that it needs a running server.

Each page has a search field for names, paths, descriptions, and values. Matching unconfigured fields open automatically, and findings remain visible while searching. Clear the search or change pages to restore the full list.

**Search** in the header, or `Cmd+K` / `Ctrl+K`, searches every section of the loaded overview, findings included, without asking the server. Arrow keys move through the results, `Enter` opens one, and `Esc` closes the search and returns focus. Opening a result puts a link to that item, setting, or finding in the address bar; the link works after a reload and in browser history. A link to an entry the configuration no longer has says so and selects nothing else. Links name entries within the configuration the server or snapshot shows, so they only work for someone who can open that same address.

The interface shares the website's warm neutral palette and rust accent in light and dark themes. Source paths sit below descriptions, edit controls stay alongside their rows, and lifecycle stages form a numbered timeline. All five page links remain visible on narrow screens. **Jump to** links move to a source group and focus its heading while keeping the current search.

In Settings, **View and edit** changes both the displayed values and the save destination. Shared shows the shared defaults; Personal shows your overrides and labels inherited shared settings. Personal values take priority for you. Rows with different values show both, such as **Shared: Off · Personal: On**. Built-in counts follow that view. A personal override disables the corresponding shared control and offers **View personal settings**. Reset actions appear in Personal. The config snapshot and **What each tool gets** show the combined configuration used for sync.

Authority uses the same scope selector. **Add rules** or **Edit rules** changes Allow, Ask first, or Always blocked, one rule per line. **Preview changes** shows the destination and exact file diff; **Save rules** saves that source file. Run the displayed sync command to apply it. A personal list replaces the same list in its shared spec, with a comparison when they differ and **Reset to shared rules** to inherit again. Multiple editable settings files require an explicit choice. Default permission mode and protected paths remain read-only. See [permission editing](../docs/site/content/docs/cli-reference/inspect.md#permission-editing) for details.

Source lists use aligned rows. Unconfigured fields stay in a disclosure. Lifecycle stages start closed, with names, counts, and descriptions visible. Search opens matching stages; a linked hook or finding opens its stage and receives focus. Clearing search closes the stages, and open states are not saved. Support groups stack on mobile. The findings summary jumps to the full list; errors remain above the controls.

Change previews keep Apply and Cancel at the top. Affected tools, changed files, and review notes start collapsed. Notes group by tool, scroll within a bounded area, and have a **Copy notes** action for their full text. Pending source changes remain visible above those details.

Values render by type: a list as chips, a boolean as a switch, anything
else as text. Project built-in switches preview and apply changes across the configured targets, with one shared undo for the last successful apply. Previews check source and output files; unrelated local working folders do not block them. Global built-ins and boolean switches save their source files after validation. See [web changes and undo](../docs/site/content/docs/cli-reference/inspect.md#serve) for prerequisites and recovery. Lifecycle shows each tool that
runs a stage as its icon (`src/assets/tools/`, see its README). A Markdown or YAML item has an edit icon that opens
it in the editor picked below the navigation, Cursor or VS Code (`cursor://file/<absolute path>` or `vscode://file/<absolute path>`); the browser remembers the choice and Cursor is the default. Narrow screens hide the choice, since a phone cannot open a desktop editor. Each item carries the lint and doctor findings that name
its file. A finding whose file is on the server's disk opens at its line when it has one, from the `openPath` the server sends. Every open link has Copy path beside it, since a browser cannot tell whether the editor opened. Mock and static data name no local files, so they show paths without open links. The JSON overview also reports whether generated files match
what `sync` would write.

`src/contract.ts` mirrors the Go types in `internal/overview/types.go`
and `internal/authority/types.go`. Update them together. `src/builtin-change.ts` validates the change responses from `internal/cli/serve_builtin_changes.go`.

See [the local UI design system](DESIGN.md) for layout, colors, and component conventions.

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
go run ./cmd/agnostic-ai view --dir internal/authority/testdata/demo-project --json \
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
| `q`       | The section search |
| `entry`   | The linked item, setting, or finding, from `src/model/identity.ts` |
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

The live suite also checks Handoff then Caveman for Claude and Codex, pending source updates, shared undo after reload and filtering, and conflicts that preserve later source and output edits.

## Source layout

- `src/contract.ts` the JSON contract, mirrored from Go
- `src/api.ts` loading from the server, the mock fixture, or a static snapshot
- `src/model/` URL state and normalization
- `src/components/` the shell, the section page, and the JSON inspector
- `src/styles.css` every colour as a token on `:root`, with a light theme
  under `[data-theme="light"]` and reduced-motion rules
