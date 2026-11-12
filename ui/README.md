# Local configuration UI

Run `agnostic-ai view` from a project or any folder inside it to open its configuration in a browser. Outside a project, it opens user configuration; `--global` selects that configuration explicitly. Installed binaries include the client.

| Section | Shows |
| --- | --- |
| Conversations | Recent Claude Code, Codex, and Cursor chats, with actions to reopen them |
| Context | Context size, instructions, rules, agents, and skills |
| Authority | Permission rules, default mode, and protected paths, with edit previews |
| Tools | MCP servers and their cached tool listings |
| Lifecycle | Session stages and their hooks, with an Add hook form |
| Settings | Built-ins, coding tools, config values, preferences, and the editor used by Open links |

See [DESIGN.md](DESIGN.md) for visual rules, [the API reference](../docs/internal/authority-view.md) for the server contract, and [the CLI reference](../docs/site/content/docs/cli-reference/serve.md#serve) for editing and recovery.

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

Open `http://127.0.0.1:5173`. Vite forwards `/api` to `http://127.0.0.1:4310`, preserving the original Host header. Set `AGNOSTIC_AI_API_URL` to use another backend address. Other browser origins need an exact `--allow-origin` on the Go server.

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

**Mock data.** Add `?mock=1` to the URL. The client reads `src/fixtures/overview.mock.json`, the server's output for the demo project, with the checkout's absolute path replaced so no machine path is committed. Regenerate it after a contract change:

```sh
go run ./cmd/agnostic-ai view --dir internal/authority/testdata/demo-project --json \
  | sed "s|$PWD/|/path/to/agnostic-ai/|g" > ui/src/fixtures/overview.mock.json
```

**Static snapshot.** Add `?source=<directory or URL>`, for example `?source=./snapshot`. The client reads `<source>/overview.json` with plain `fetch`. A relative source resolves against the page; an absolute one must be http(s). A snapshot or the mock fixture is read-only: the notice at the top of each page says that opening files, rechecks and switches need `agnostic-ai view`. A source that does not load shows **Could not load the snapshot**, with the URL and status under **Details** and **Retry**. Capture one from a running server:

```sh
mkdir -p snapshot
curl -s http://127.0.0.1:4310/api/views/overview > snapshot/overview.json
```

For static hosts such as GitHub Pages, set a default source so visitors need no query string:

```sh
VITE_DEFAULT_SOURCE=./snapshot npm run build
```

The build uses relative asset paths, so `dist/` runs from any directory.

## URL state

| Parameter | Meaning |
| --------- | ------- |
| `view`    | `conversations` (default with a server or mock data), `context` (default for a static snapshot), `authority`, `tools`, `lifecycle`, `settings`, `health` |
| `tab`     | The selected source group within Context, Tools, or Settings |
| `q`       | The section or Conversations search |
| `entry`   | The linked item, setting, or finding, from `src/model/identity.ts` |
| `mock`    | `1` to read the bundled fixture |
| `source`  | A static snapshot directory or URL |

Overview JSON, at the foot of the rail (in Menu on phones), opens the JSON inspector with the overview document and the command that prints it (`agnostic-ai view --json`). `Esc` closes it.

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

**Model tests** (`src/model/*.test.ts`) cover URL state, source parsing, and fixture conformance to the contract. Set `OVERVIEW_FIXTURE` to server output, using a path relative to `ui/`.

**Fixture browser tests** (`e2e/fixture/`) check rendering, themes, keyboard controls, and mocked edit responses for success and refusal.

**Live browser tests** (`e2e/live/`) use the built Go server, embedded client, and temporary project and user sources. They check real API requests, saved edits after reload, browser boundaries, a local fake MCP server, and the Vite proxy. They never access the user's configuration.

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

Use `E2E_CHROMIUM_PATH` for an existing Chrome installation, or install the browser matching the pinned Playwright version with `npx playwright install chromium` from `ui/`. CI installs the pinned browser before running both suites.

The live suite also checks Handoff then Caveman for Claude and Codex, pending source updates, shared undo after reload and filtering, and conflicts that preserve later source and output edits.

## Source layout

- `src/contract.ts` the JSON contract, mirrored from Go
- `src/api.ts` loading from the server, the mock fixture, or a static snapshot
- `src/model/` URL state and normalization
- `src/components/` the shell, the section page, and the JSON inspector
- `src/styles/`: stylesheets imported in order by `index.css`. `tokens.css` defines colors on `:root` and `[data-theme="light"]`. `shell.css`, `components.css`, and area stylesheets each contain their media, reduced-motion, and forced-colors rules.

## Navigation and search

Conversations is the default page with a server or mock data. Static snapshots open Context because they contain no chats. `defaultView` in `src/model/url.ts` chooses the default, which links omit: Conversations is `/`, Context is `?view=context`.

The desktop rail shows the project, Setup health, and each page's question and summary (`sectionSummary` in `src/model/rail.ts`). Its foot holds Search, the configuration switch, theme, GitHub links, version, and Overview JSON. At 900px and below, a top bar and one row of page links replace the rail; Menu opens its contents in a side sheet. Mock data and snapshots display one notice at the top of the page.

The version comes from `/api/health`. Releases link to their GitHub notes; checkout builds show `<branch>@<sha>` without a link. Mock data and snapshots show no version.

Each section starts with a summary sentence and a facts line (`src/model/verdict.ts`). The facts identify unsupported configuration, such as "Kiro ignores permissions", and count findings on the section and its rows. Errors appear before the tabs; **Review** opens the warnings list or focuses the first affected row. **Tool support** opens a dialog with `supportSummary`, tool names and logos, and links to each type's tab and heading (`entry=kind:<kind>`). **Docs** opens the relevant documentation.

The section search matches names, paths, descriptions, and values. It opens matching groups and unconfigured fields, shows match counts only while filtering, and leaves findings visible. Clearing it or changing pages restores the full list. Empty groups explain the type, list supporting tools, and offer **Copy prompt** from `health.gaps`; a search with no results says **No matches.**

**Search** in the rail foot or mobile top bar, also `Cmd+K` / `Ctrl+K`, searches all loaded sections and findings without a server request. Arrow keys select a result, `Enter` opens it, and `Esc` closes search and restores focus. Results link to an item, setting, or finding; links survive reload and browser history. A missing entry is reported without selecting another. These links require access to the same server or snapshot.

Context, Tools, and Settings use tabs. Context starts with context size, then guidance, rules, agents, and skills. Tools separates MCP servers, servers found in tools, and environments. Settings separates built-ins, coding tools, preferences, config files, and the editor. Authority keeps permissions together; Lifecycle uses a timeline.

Tabs support Left/Right, Home, and End. The `tab` URL parameter survives refresh and history but clears when changing pages. Search covers every tab and opens a matching group; linked results open their owning tab. Tab tooltips list the types they contain. Counts appear once and only above zero. Switching tabs keeps drafts, previews, and context-size state. The tab row scrolls on narrow screens without widening the page. Findings and tool-support controls remain outside the panels.

## Conversations

The page reads `GET /api/views/conversations?editor=<id>` using the editor chosen in Settings. The first page includes all open chats and the past week. Each response's `next` supplies the next `before=<time>`, skipping empty weeks.

An `IntersectionObserver` near the list's end loads the next week, one request at a time. **Load the week before** provides the same action by click or keyboard and works without the observer. While loading it reads **Loading the week before…**. A failed request keeps the loaded rows and offers **Try again** for that week; no `next` ends the list with **That's every conversation.** Rows merge by tool and id, with the later copy winning, newest first, and open chats on top.

`conversationsVerdict` reports open chats, those waiting for you and those with no recent activity, or when the last chat ran. The facts line counts loaded chats. Tool filters show **All** and an `aria-pressed` toggle for each detected store, with its loaded count and a tooltip such as "12 of 66 Codex chats loaded." Pressing the selected tool restores All. Missing stores are omitted.

Chats open in a process or editor window appear under **Open now**, newest first. Others group by the last user or agent message in local time: Today, Yesterday, This week, Earlier. Each row shows a title; the agent's latest reply, as one line under the title (local Claude Code, Codex and Cursor chats only); tool, folder, branch, and time; and the last prompt unless it repeats the title. The tool's tooltip names the app, such as Terminal or Desktop app. Shell input such as `<bash-input>gl</bash-input>` renders as `! gl` in monospace.

Open chats show **Working** or **Waiting for you** in amber when the tool reports activity, as Claude Code does. A local Codex wait is read from 5 minutes of silence, so its row carries `quietSince` and shows **No activity for 7m** instead. `stateBadge` maps the row's `state` to **New reply** when a message arrived since the last open, or **Archived**; other states need no badge. The server excludes SDK sessions and Codex exec runs. Withheld fields read **Hidden: may hold a credential**. A title may be a name chosen in the companion; the server sends it as `title`, so the page and its search need no change.

Search sends `q` after a 300 ms typing pause and covers chats of every age, including unloaded weeks. Every word must match the displayed title, folder, branch, last prompt, latest reply, tool, or app. Results page by week and report the total match count, with **Searching…** while loading. Tool filters narrow the returned rows; scrolling still loads more matches. Only the unfiltered list is cached for a quick return. Mock mode applies the same matching rule.

The primary action, such as **Open in Terminal** or **Switch to Ghostty**, becomes a filled button on hover or keyboard focus and stays filled for open chats. **Copy command** sits beside it, or in the ⋯ menu on touch screens and phones. Opening posts `{tool, id, editor}` to `POST /api/conversations/open`; the row displays the server's reply. For editor extensions, the server first opens the chat's folder, waiting about one second for an existing window or up to five for a new one. If opening fails and the server supplies a command, **Copy command** offers it.

A chat whose `parent` is in the same group sits right after it, indented, and screen readers hear "Started from" and the parent's title. Children of children sit under the same parent, oldest first, one level deep. A parent in another group, on a week not loaded yet, or filtered out leaves the child as a normal row; when the parent's week loads, the child moves under it. A loop typed by hand draws each row as a normal one.

Helpers live in `src/model/conversations.ts`. Mock mode uses `src/fixtures/conversations.mock.json` as one week and sends no open requests. Snapshots explain that Conversations needs a running server.

## Findings and setup health

Finding cards show **Needs a fix** or **Worth a look**, the owning section when shown elsewhere, a title, and a source path. **Copy prompt** copies the complete diagnostic. **Details** contains the original message, suggested fix, explanation, check code, exact prompt, editor link, **Copy path**, and **Recheck** (`src/components/Findings.tsx`). Long titles collapse to three lines with **Show full message**. Findings in the same file with the same title also show their raw messages to distinguish them.

With a server, **Recheck** refreshes the overview and compares finding ids, which ignore line numbers. A failed refresh or `lint-incomplete` result never counts as a fix. A disappearing doctor finding is marked not confirmed because doctor skips unreadable files.

The rail's Setup health row shows **Needs attention** or **All clear**, a status dot, and counts such as "1 to fix, 12 you could add" (`src/model/health.ts`, `src/model/rail.ts`). The mobile top bar shows severity counts such as "1E 3W". Both link to `?view=health`.

Setup health starts with `healthVerdict`, finding and suggestion counts, and whether the data was checked just now or came from a snapshot. It offers **Docs** and, with a server, **Recheck**. Findings group under **Fix first** and **Worth a look**, then by section and item, using the same cards as section pages. Item and section links open the affected location.

With two or more findings, **Copy all fixes** combines them into one prompt, errors first, with shared guidance once. Checkboxes on findings and suggestions (`src/components/pick.tsx`) let users select a subset. Selecting any shows a bottom bar with the count, **Copy prompt**, and **Clear**. `healthFixPrompt` orders selected errors first, then other findings and suggestions, with shared guidance once. Fixed findings leave the selection after a recheck.

**You could add** lists `health.gaps`, such as "Skills: No skill yet. 2 of your tools can use one." (`gapLine`). Names link to their sections; **Copy prompt** appears on hover or focus, or in the mobile ⋯ menu. Phones show four suggestions before **Show N more**. Suggestions never count as findings.

`health.improvements` supplies a bar per supported feature, filled when in use, and a line such as "7 of 9 features your tools support are in use" (`improvementsLine`). The track hides without improvements data; the suggestion list hides without gaps. Both use the loaded overview and work in snapshots, which omit Recheck.

## Context and source files

**Context size** reads `GET /api/views/context?target=<tool>&file=<path>`. It is the first tab for projects with a coding tool, using a server or mock data. Snapshots, global configuration (`global` in the overview), and projects without a coding tool open the first populated tab instead.

The report matches `explain --context` for one target. **Check against a file** includes scoped rules when the tool supports them. Startup and on-demand counts come first, followed by a comparison with `startupWordBudget` (`lint.instructions-words`) and **Change budget** (`src/model/context-verdict.ts`). **Next** offers the largest startup source, **Copy command** for explain, and **Copy prompt** for a startup review.

**How this is counted** explains load conditions and the tool's default model and effort from Settings (`src/model/model-line.ts`). Sources rank by share; duplicated startup passages appear only when found, with edit links and **Copy prompt**. Each source's **What can I do?** explains possible changes. **Copy all prompts** combines advice for actionable sources, startup first (`contextAllAdvicePrompt`). Advice comes from `src/model/context-advice.ts`.

**Read again** refreshes the report and labels previous counts **Before**. Source types use plain names such as instructions file, tool wrapper text, and skill name and description; prompts retain raw categories. `target` and `file` persist in the URL. Mock mode uses `src/fixtures/context.mock.json`; snapshots require a server for this report.

Context rows for instructions, rules, agents, skills, commands, review instructions, and ignore files offer **Show content** with a server. The first open fetches `GET /api/views/source?path=<openPath>` and caches by path. Frontmatter appears as chips, followed by plain text, with a truncation notice for bodies over 256 KB. The server serves only files listed by the loaded configuration. Search excludes file contents; snapshots and mock mode hide the toggle.

## Lifecycle

The timeline shows stages with hooks; **Show all stages** includes empty stages. Stages start closed, with names, descriptions, and nonzero counts. Turned-off hooks are counted separately. Without hooks, the page shows the shared empty state and a plain list of stages and tools. Tool-specific events have their own group.

Search opens matching stages. A linked hook or finding opens its stage and receives focus. Clearing search closes stages; open states are not saved. **Expand all** and **Collapse all** affect stages without changing the disclosures inside each hook.

Hooks show their description, origin, and a **Command** disclosure, or **Details** when no command exists (`src/components/HookDetails.tsx`). Fields come from the overview (`on`, `event`, `match`, `matcher`, `type`, `command`, `timeout`, `disabled`), so they also work in snapshots. Search includes these fields and the command.

**Full file** uses the source endpoint to show hook fields without raw text. Files that may contain credentials are withheld. Built-ins, snapshots, and mock mode hide this action. Personal hooks label it **Fields this file sets**: it reads that file alone, while overview fields show the merged hook.

With a server, **Add hook** previews and writes one portable hook spec, then asks the user to run sync. Built-in hooks link to Settings. Tool icons come from `src/assets/tools/`; its [README](src/assets/tools/README.md) records their sources and licenses.

## Editing

Settings and Authority use one scope selector per page: **Project | Local**, or **Global | Local** for user configuration. It changes displayed values and the save destination. Project and Global show their own values; Local shows git-ignored overrides and labels inherited values. Local values win. Differences show both values, such as **Project: Off · Local: On**; built-in counts follow the selected scope.

A local override disables the project control and offers **View local settings**. Reset actions appear in Local. API audiences remain `shared` and `personal`. The Config files tab hides the selector; its snapshot and **What each tool gets** show the combined configuration used by sync.

Authority's **Add rules** and **Edit rules** accept one rule per line for Allow, Ask first, or Always blocked. Saved rules also have **Edit** and **Remove**. **Preview** shows the destination and exact diff; **Apply** saves the source file. The displayed sync command applies it to tools. Local lists replace project lists and show a comparison, with **Reset to the project rules** to inherit again. Multiple editable settings files require a choice.

**Default permission mode** lists Claude Code's accepted mode names. It saves to the same settings file in the global view; projects show it read-only with **Open global settings**. **Protected paths** accepts paths, a decision, and a reason, with preview and save. Local blocks start from project values; the global view is read-only. See [permission editing](../docs/site/content/docs/cli-reference/serve.md#permission-editing).

Coding-tool switches save `targets` through `POST /api/edit/targets`: `{audience, name, enabled, expected}` or `{audience: 'personal', reset: true, expected: [names]}`. Turning a tool off requires confirmation. The overview's `tools` catalog includes `source` (`default`, `shared`, `personal`) and `items`. Helpers live in `src/model/coding-tools.ts`; `targetDocPath` in `src/model/documentation.ts` supplies docs links.

Values render as chips for lists, switches for booleans, and text otherwise. Native settings group keys by tool. Project built-in switches preview and apply changes across configured targets, with one shared undo for the last successful apply. Previews check source and output files; unrelated local working folders do not block them. Global built-ins and boolean switches validate and save source files. See [prerequisites and recovery](../docs/site/content/docs/cli-reference/serve.md#serve).

Change previews keep Apply and Cancel at the top. Affected tools, changed files, and review notes start collapsed; notes group by tool, scroll within a bounded area, and offer **Copy notes** for the full text. Pending source changes stay visible above these details.

Markdown and YAML items open in the editor selected in Settings: Cursor (default), VS Code, VS Code Insiders, VSCodium, Zed, or a custom template using `{path}` and `{line}`. Links use `<scheme>://file/<absolute path>:<line>`. Choices persist in `agnostic-ai.editor` and `agnostic-ai.editor-template`. The Editor tab requires a server.

Items carry lint and doctor findings for their files. Editor links use the server's `openPath` and line when available. **Copy path** accompanies each link because the browser cannot confirm that the editor opened. Mock and static data show paths without editor links. The overview also reports whether generated files match sync output.

## Contract

`src/contract.ts` mirrors `internal/overview/types.go` and `internal/authority/types.go`; update them together. `src/builtin-change.ts` validates responses from `internal/cli/serve_builtin_changes.go`.

The client uses the [Mineral design system](DESIGN.md), including light and dark themes. `src/styles/tokens.test.ts` checks its documented colors and contrast.
