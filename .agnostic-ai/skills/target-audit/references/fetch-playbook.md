# Fetch playbook

Read this when a `docfetch.tsv` row is `failed`, or when its mode is `app-shell`, `soft-404`, or `redirected`. Every other row already carries usable text and needs nothing from this file.

`scripts/docfetch.sh` runs the mechanical part of this ladder on every URL. What stays here is the judgement, the vendor-specific routes the script cannot guess, and the failures that have cost real audit time.

## What each mode means

`html` is a normal page; the hash covers its visible text only, so a nonce or a rebuilt script bundle does not read as a documentation change. `markdown-mirror`, `llms-txt`, `raw-github`, and `text` are raw sources, hashed as fetched. `json` is hashed with its object keys sorted, so an API that reorders a map between fetches does not read as a change. `github-api` hashes only the release tags and dates, so download counters do not churn. `meta-refresh` and `reader-proxy` are successful recoveries, and the row's `final_url` says where the content actually came from.

`app-shell` means the page served 200 with almost no visible text and every automatic fallback failed. `soft-404` means the body announces a missing page behind a 200. `redirected` means the fetch landed somewhere other than the path requested. `failed` means no usable bytes at all. None of these are clean checks.

Saved bodies are under `<run>/pages/<target>/`. Read the saved copy rather than re-fetching.

## Manual ladder

Work in this order, cheapest first, and stop at the first route that serves the page's real text.

`curl -sD - <url> | head -c 400` shows the status, the headers, and the first bytes together. It is the fastest way to tell a real page from a stub.

A short body containing `http-equiv="refresh"` is a client-side redirect. WebFetch follows HTTP redirects but not this tag, so a moved page reads as a live short page. Read the `url=` value out of the tag and fetch that instead.

Append `.md` to the docs path. Many vendors serve a clean markdown mirror that way.

Fetch `llms.txt` on the docs host. It often indexes every page in one call, and on a Mintlify site it lists a changelog that the navigation hides.

Find the docs source repo on GitHub and read the markdown directly. `gh api search/code` scoped to the docs path finds which page names a string.

Call the route the single-page app calls itself. A `/api/` path visible in the page source usually returns the same content as JSON.

Extract the server-rendered payload. Many sites ship CMS-authored content inside a `<script>` block such as `window._ROUTER_DATA`, `window.__NEXT_DATA__`, or `__INITIAL_STATE__`. WebFetch truncates these because they run to megabytes, so the page reads as empty while the full text sits in the response. Fetch to disk with `curl`, locate the payload, and parse it.

Retry through `https://r.jina.ai/<url>` when the host blocks the fetch rather than the path.

Only after all of these fail is `unconfirmed` the honest answer. Report it as a research limit, never as a clean target.

## A 200 is not a correct answer

A byte count is not proof of a successful fetch. Grep the body for a string you expect before trusting it. `ampcode.com/manual` returns 25 KB of app shell carrying none of the manual's text.

A soft 404 passes a status check. `cursor.com/docs/commands` serves 200 with the full docs chrome and a not-found graphic in the body. Grep for the page's own heading before recording it as live.

A blanket redirect proves nothing about a specific page. When `cursor.com` was unreachable from an audit sandbox, both fallback hosts 308ed every path to the bare `/docs` index, which is exactly the setup for concluding that live pages are gone.

A moved URL can also land on something unrelated. When a fetch succeeds but the content does not match the topic, that is a `docs-moved` finding, not an absent surface.

Re-check every negative with `curl -sL` before reporting it. A fetch that contradicts a prior audit is more often a fetch failure than a vendor change.

When an entry names a file, a symbol, or a page as the thing to trust, check that the name still exists before trusting what it says. A URL health check cannot catch a live page whose named authority is dead.

## Vendor-specific routes

Appending `.md` to a docs path works on factory, qoder, augment, openhands, and cline (all Mintlify), on antigravity and cursor (a Vercel raw route, not Mintlify), and on kiro, whose pages link to it in-page as "View as Markdown". It also works on learn.chatgpt.com, docs.devin.ai, docs.github.com, docs.warp.dev, and opencode.ai. It does not work on kilo or trae. On antigravity it covers the `/docs/` tree only; `antigravity.google/changelog.md` 404s.

amp: `llms.txt` serves every page at `https://ampcode.com/docs/markdown/<path>`, and the older `/docs/<path>/markdown` form 404s. `https://ampcode.com/llms.txt` indexes every docs page in one call and states the rule itself. The page count moves, so re-count it rather than trusting a number recorded anywhere.

junie: `https://junie.jetbrains.com/docs/HelpTOC.json` returns the full page list in one call. That is how three uncited pages were found.

trae: neither WebFetch nor `.md` works; `ide/rules.md` returns 1.8 MB of SPA shell. Content is server-side inside `window._ROUTER_DATA` as a Quill delta, where every token of every code example is its own tagged run, so the reconstruction is exact rather than approximate. Curl to disk, brace-match the object, and collect `ops[].insert` per `zoneId`, since table cells arrive as separate zones. Per-page `updated_at` lives in the same object under `busStructure`, which dates a page without diffing its text. The changelog is separate and easy: `https://www.trae.ai/api/changelog` returns JSON.

kiro: Next.js server-rendered, so plain curl plus a tag strip gives full text. WebFetch truncates the 280 KB pages. The host also answers 403 to some clients, and the reader proxy gets through.

kilo: `kilo.ai` is client-rendered, `.md` append 404s, and so do `kilo.ai/api/raw-markdown?path=...` and `kilo.ai/api/llms.txt` on a direct GET even though both exist as route files. The `Kilo-Org/kilocode` mirror at `packages/kilo-docs/pages/**` is the only route.

continue: no `.md` mirror and no `llms.txt`. Use `continuedev/continue` at `docs/customize/deep-dives/*.mdx`.

qoder: `docs.qoder.com/llms.txt` needs `curl --compressed`, or the response is not valid UTF-8 and reads as garbage.

## Lessons worth one line each

Kiro's configuration reference moved and left a 595-byte meta-refresh stub at the old URL. Two audits read that stub and shipped a coverage note calling the tool vocabulary unconfirmed when it had been fully documented the whole time.

Trae's MCP page 302s to a marketing page, which is why two consecutive runs concluded the path was undocumented.

Trae's MCP schema was settled from the server-rendered payload only after `.md`, `llms.txt`, a docs repo, and the SPA's own API had all dead-ended.

Both kilo breaking findings of the 2026-08-01 run were proven from the docs source repo, not from the rendered site.

## Corpus evidence

When a corpus of real config files is the only evidence available, separate the files the vendor's own tool produced from the files another tool wrote into the same folder. A Trae MCP file carrying `autoApprove` and Cline-shaped tool names at the wrong path is evidence about Cline. Averaging a contaminated corpus produces a schema no vendor actually accepts.
