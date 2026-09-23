"use strict";

const SAMPLES = {
  rule: `---
name: conventional-commits
description: Always use Conventional Commits format.
globs: "**/*"
alwaysApply: true
---

Use Conventional Commits for every commit:

- \`feat:\` new feature
- \`fix:\` bug fix
- \`docs:\` documentation only
- \`refactor:\` code change without feature/fix
- \`test:\` tests only
- \`chore:\` build, deps, CI

Subject line under 72 chars. Body explains why, not what.
`,
  agent: `---
name: code-reviewer
description: Reviews diffs for bugs, style, and security issues.
model:
  claude: claude-opus-5-5
  cursor: gpt-6-sol
tools:
  - Read
  - Grep
  - Bash
---

Review the current branch's diff. Surface bugs, security issues, and style violations.
Cite file paths and line numbers. Be terse; lead with the highest-impact finding.
`,
  skill: `---
name: api-docs
description: Generate OpenAPI documentation from route handlers.
---

When asked to document an API:
1. List every route handler.
2. Extract method, path, params, and response schema.
3. Emit an OpenAPI 3.1 YAML stub.
`,
  hook: `name: pre-commit-format
event: pre-commit
command: make fmt
description: Auto-format staged files before each commit.
`,
  mcp: `name: github
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-github"
env:
  GITHUB_PERSONAL_ACCESS_TOKEN: \${GITHUB_TOKEN}
`,
  command: `---
name: review
description: Review the current branch diff for bugs, style, and test coverage.
---

Review the diff since the last commit:

1. List files changed.
2. Surface bugs, edge cases, and style violations.
3. Check test coverage for new behavior.

Cite \`file:line\` for every finding. Lead with highest-impact issues.
`,
  settings: `name: defaults
permissions:
  allow:
    - Bash(go test:*)
  deny:
    - Bash(rm:*)
  ask:
    - Bash(git push:*)
model: claude-opus-4-8
`,
  review: `---
name: backend-review
scope: backend
---

Flag handlers that access the database without going through a repository.
`,
  environment: `name: development
install: go mod download
terminals:
  - name: dev
    command: go run ./cmd/agnostic-ai
`,
  ignore: `---
name: private-files
---

# Secrets and build artifacts the agent should never read
*.env
secrets/
dist/
`,
};

/* One line per spec kind, plus the section of the spec-format page that
   documents it. The dropdown is where most people meet the full list, so
   every option says what it is and links to the reference. */
const KINDS = {
  agent: {
    summary: "A named subagent with its own instructions, tool list, and model.",
    anchor: "agents",
    docLabel: "Spec format: agents",
  },
  skill: {
    summary: "A procedure the agent loads on demand, with its reference files.",
    anchor: "skills",
    docLabel: "Spec format: skills",
  },
  rule: {
    summary: "Always-on project conventions, narrowed to paths with globs.",
    anchor: "rules",
    docLabel: "Spec format: rules",
  },
  hook: {
    summary: "A command the tool runs on a lifecycle event, such as before a commit.",
    anchor: "hooks",
    docLabel: "Spec format: hooks",
  },
  mcp: {
    summary: "An MCP server: transport, command, arguments, environment.",
    anchor: "mcp-servers",
    docLabel: "Spec format: MCP servers",
  },
  command: {
    summary: "A slash command the tool lists in its command picker.",
    anchor: "commands",
    docLabel: "Spec format: commands",
  },
  settings: {
    summary: "Portable permission rules and the default model.",
    anchor: "settings",
    docLabel: "Spec format: settings",
  },
  review: {
    summary: "Guidance for a code-review bot. Cursor Bugbot and Goose read it.",
    anchor: "reviews",
    docLabel: "Spec format: reviews",
  },
  environment: {
    summary: "How an agent boots the dev environment: install, services, terminals.",
    anchor: "environments",
    docLabel: "Spec format: environments",
  },
  ignore: {
    summary: "Gitignore-syntax patterns an agent must not read or index.",
    anchor: "ignore",
    docLabel: "Spec format: ignore",
  },
};

const SPEC_FORMAT_URL = "../docs/spec-format/";

// A demo renders a few well-known targets; the rest link to the full target list.
const DEMO_TARGETS = ["claude", "codex", "copilot", "gemini", "cursor"];
const TARGETS_URL = "../docs/targets/";

const $ = (id) => document.getElementById(id);
const els = {
  status: $("status"),
  app: $("app"),
  source: $("source"),
  kind: $("kind"),
  sample: $("sample"),
  kindSummary: $("kind-summary"),
  kindDoc: $("kind-doc"),
  tabs: $("tabs"),
  files: $("files"),
  fileSelectWrap: document.querySelector(".file-select"),
  filemeta: $("filemeta"),
  content: $("content"),
  copy: $("copy"),
};

let renderResults = [];
let currentTarget = null;
let currentFile = null;
let capabilityByTarget = new Map();
let moreTargets = null;

/* ─── Status ─── */

function setStatus(msg, isError) {
  if (msg == null) {
    els.status.hidden = true;
    return;
  }
  els.status.hidden = false;
  els.status.textContent = msg;
  els.status.classList.toggle("error", !!isError);
}

/* ─── Targets ─── */

function loadTargets(capabilities) {
  capabilities.forEach(({ name, supports }) => capabilityByTarget.set(name, new Set(supports)));
  const hidden = capabilities.length - DEMO_TARGETS.length;
  if (hidden <= 0) return;
  moreTargets = document.createElement("a");
  moreTargets.className = "more-targets";
  moreTargets.href = TARGETS_URL;
  moreTargets.textContent = `+${hidden} more`;
  moreTargets.setAttribute("aria-label", `+${hidden} more targets in the CLI`);
  moreTargets.title = `This demo shows ${DEMO_TARGETS.length} of ${capabilities.length} targets. The CLI supports all of them.`;
}

function supportsKind(target, kind) {
  return capabilityByTarget.get(target)?.has(kind) || false;
}

/* ─── Kind description ─── */

function updateKindHint() {
  const kind = els.kind.value;
  const info = KINDS[kind];
  if (!info) return;
  els.kindSummary.textContent = info.summary;
  els.kindDoc.href = `${SPEC_FORMAT_URL}#${info.anchor}`;
  els.kindDoc.textContent = info.docLabel;
}

/* ─── Sample ─── */

function sampleKind(source) {
  return Object.keys(SAMPLES).find((kind) => SAMPLES[kind] === source) || null;
}

function updateSampleAction() {
  const kind = els.kind.value;
  const verb = sampleKind(els.source.value) === kind ? "Reset" : "Load";
  const label = `${verb} ${kind} sample`;
  els.sample.textContent = label;
  els.sample.setAttribute("aria-label", label);
}

function buildSampleAction() {
  updateSampleAction();
  els.sample.addEventListener("click", () => {
    els.source.value = SAMPLES[els.kind.value];
    updateSampleAction();
    scheduleRender();
  });
}

/* ─── Output ─── */

function bytesLabel(s) {
  const n = new Blob([s]).size;
  if (n < 1024) return `${n} B`;
  return `${(n / 1024).toFixed(1)} KB`;
}

function renderTabs() {
  els.tabs.innerHTML = "";
  const byTarget = new Map();
  renderResults.forEach((f) => {
    if (!byTarget.has(f.target)) byTarget.set(f.target, []);
    byTarget.get(f.target).push(f);
  });

  if (!byTarget.has(currentTarget)) {
    currentTarget = DEMO_TARGETS.find((t) => byTarget.has(t)) || null;
  }

  const kind = els.kind.value;
  DEMO_TARGETS.forEach((t) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.role = "tab";
    const count = byTarget.get(t)?.length || 0;
    btn.textContent = count > 1 ? `${t} · ${count}` : t;
    btn.setAttribute("aria-selected", t === currentTarget);
    if (count === 0) {
      btn.disabled = true;
      btn.title = supportsKind(t, kind)
        ? `${t} writes no file for this spec`
        : `${t} does not support ${kind} specs`;
    } else {
      btn.addEventListener("click", () => {
        currentTarget = t;
        currentFile = null;
        renderTabs();
      });
    }
    els.tabs.append(btn);
  });
  if (moreTargets) els.tabs.append(moreTargets);

  if (!currentTarget) {
    els.content.textContent = "";
    els.fileSelectWrap.hidden = true;
    els.filemeta.textContent = "";
    els.copy.disabled = true;
    return;
  }

  const files = byTarget.get(currentTarget) || [];
  if (!currentFile || !files.some((f) => f.path === currentFile)) {
    currentFile = files[0]?.path || null;
  }

  els.files.innerHTML = "";
  files.forEach((f) => {
    const opt = document.createElement("option");
    opt.value = f.path;
    opt.textContent = f.path;
    if (f.path === currentFile) opt.selected = true;
    els.files.append(opt);
  });
  els.fileSelectWrap.hidden = files.length <= 1;

  const file = files.find((f) => f.path === currentFile) || files[0];
  if (file) {
    els.content.textContent = file.content;
    els.filemeta.textContent = files.length > 1
      ? bytesLabel(file.content)
      : `${file.path} · ${bytesLabel(file.content)}`;
    els.copy.disabled = false;
  }
}

/* ─── Render pipeline ─── */

let pending = 0;
function scheduleRender() {
  const seq = ++pending;
  setTimeout(() => {
    if (seq !== pending) return;
    runRender();
  }, 80);
}

function runRender() {
  const kind = els.kind.value;
  const targets = DEMO_TARGETS.filter((target) => supportsKind(target, kind));
  let result;
  try {
    result = window.agnosticAIRender(els.kind.value, els.source.value, targets);
  } catch (e) {
    setStatus(`render failed: ${e.message || e}`, true);
    return;
  }
  const errors = result.errors || [];
  setStatus(
    errors.length ? errors.map((e) => `${e.target || "(input)"}: ${e.message}`).join(" · ") : null,
    errors.length > 0,
  );
  renderResults = result.files || [];
  renderTabs();
}

/* ─── Buttons ─── */

function currentFileObject() {
  return renderResults.find(
    (f) => f.target === currentTarget && f.path === currentFile,
  );
}

async function handleCopy() {
  const f = currentFileObject();
  if (!f) return;
  try {
    await navigator.clipboard.writeText(f.content);
    els.copy.classList.add("success");
    const original = els.copy.innerHTML;
    els.copy.innerHTML = '<span class="icon">✓</span> Copied';
    setTimeout(() => {
      els.copy.classList.remove("success");
      els.copy.innerHTML = original;
    }, 1200);
  } catch (e) {
    setStatus(`copy failed: ${e.message || e}`, true);
  }
}

/* ─── Init ─── */

async function init() {
  const go = new Go();
  let module;
  try {
    const resp = await fetch("agnostic-ai.wasm");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    if (WebAssembly.instantiateStreaming) {
      module = await WebAssembly.instantiateStreaming(resp, go.importObject);
    } else {
      const bytes = await resp.arrayBuffer();
      module = await WebAssembly.instantiate(bytes, go.importObject);
    }
  } catch (e) {
    setStatus(
      `failed to load WebAssembly: ${e.message || e}. Run \`make playground-build && make playground-serve\` locally to bundle and serve the page over HTTP (file:// won't work).`,
      true,
    );
    return;
  }
  go.run(module.instance);

  loadTargets(window.agnosticAICapabilities());
  els.source.value = SAMPLES[els.kind.value];
  updateKindHint();
  buildSampleAction();

  els.source.addEventListener("input", () => {
    updateSampleAction();
    scheduleRender();
  });
  els.kind.addEventListener("change", () => {
    if (sampleKind(els.source.value)) {
      els.source.value = SAMPLES[els.kind.value];
    }
    updateSampleAction();
    updateKindHint();
    scheduleRender();
  });
  els.files.addEventListener("change", () => {
    currentFile = els.files.value;
    renderTabs();
  });
  els.copy.addEventListener("click", handleCopy);

  els.app.hidden = false;
  setStatus(null);
  runRender();
}

init();
