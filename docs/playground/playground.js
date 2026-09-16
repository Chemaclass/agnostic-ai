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
model: sonnet
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

const DEFAULT_TARGETS = ["claude", "codex", "gemini"];
const STORAGE_KEY = "agnostic-ai-playground";

const $ = (id) => document.getElementById(id);
const els = {
  status: $("status"),
  app: $("app"),
  source: $("source"),
  kind: $("kind"),
  sample: $("sample"),
  targets: $("targets"),
  capabilitySummary: $("capability-summary"),
  tabs: $("tabs"),
  files: $("files"),
  fileSelectWrap: document.querySelector(".file-select"),
  filemeta: $("filemeta"),
  content: $("content"),
  copy: $("copy"),
  download: $("download"),
};

let renderResults = [];
let currentTarget = null;
let currentFile = null;
let capabilityByTarget = new Map();

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

/* ─── Persistence ─── */

function savePrefs() {
  const data = {
    kind: els.kind.value,
    targets: selectedTargets(),
  };
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  } catch (_) {}
}

function loadPrefs() {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY)) || {};
  } catch (_) {
    return {};
  }
}

/* ─── Targets ─── */

function buildTargetChips(capabilities, preselected) {
  els.targets.querySelectorAll("label").forEach((n) => n.remove());
  const wanted = preselected && preselected.length ? preselected : DEFAULT_TARGETS;
  capabilities.forEach(({ name, supports }) => {
    capabilityByTarget.set(name, new Set(supports));
    const id = `target-${name}`;
    const label = document.createElement("label");
    label.htmlFor = id;
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.id = id;
    cb.value = name;
    cb.checked = wanted.includes(name);
    cb.addEventListener("change", () => {
      savePrefs();
      scheduleRender();
    });
    const span = document.createElement("span");
    span.textContent = name;
    const supportStatus = document.createElement("span");
    supportStatus.className = "sr-only support-status";
    label.append(cb, span, supportStatus);
    els.targets.append(label);
  });
  updateCapabilityState();
}

function selectedTargets() {
  return Array.from(
    els.targets.querySelectorAll('input[type="checkbox"]:checked'),
  ).map((cb) => cb.value);
}

function supportsKind(target, kind) {
  return capabilityByTarget.get(target)?.has(kind) || false;
}

function updateCapabilityState() {
  const kind = els.kind.value;
  let supportedCount = 0;
  els.targets.querySelectorAll("label").forEach((label) => {
    const input = label.querySelector('input[type="checkbox"]');
    const supported = supportsKind(input.value, kind);
    if (supported) supportedCount += 1;
    label.classList.toggle("unsupported", !supported);
    label.title = supported
      ? `${input.value} supports ${kind} specs`
      : `${input.value} does not support ${kind} specs and will be skipped`;
    label.querySelector(".support-status").textContent = supported
      ? `, supports ${kind}`
      : `, does not support ${kind}`;
  });
  els.capabilitySummary.textContent = `${supportedCount} of ${capabilityByTarget.size} targets support ${kind} specs. Unsupported selections have dashed outlines and are skipped.`;
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

  if (byTarget.size === 0) {
    els.content.textContent = "";
    els.fileSelectWrap.hidden = true;
    els.filemeta.textContent = "";
    els.copy.disabled = true;
    els.download.disabled = true;
    return;
  }

  const targets = Array.from(byTarget.keys()).sort();
  if (!targets.includes(currentTarget)) currentTarget = targets[0];

  targets.forEach((t) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.role = "tab";
    const count = byTarget.get(t).length;
    btn.textContent = count > 1 ? `${t} · ${count}` : t;
    btn.setAttribute("aria-selected", t === currentTarget);
    btn.addEventListener("click", () => {
      currentTarget = t;
      currentFile = null;
      renderTabs();
    });
    els.tabs.append(btn);
  });

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
    els.filemeta.textContent = `${file.path} · ${bytesLabel(file.content)}`;
    els.copy.disabled = false;
    els.download.disabled = false;
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
  const selected = selectedTargets();
  if (selected.length === 0) {
    renderResults = [];
    setStatus("Pick at least one target.");
    renderTabs();
    return;
  }
  const kind = els.kind.value;
  const targets = selected.filter((target) => supportsKind(target, kind));
  const skipped = selected.filter((target) => !supportsKind(target, kind));
  if (targets.length === 0) {
    renderResults = [];
    setStatus(`None of the selected targets support ${kind}. Pick a target shown at full contrast.`);
    renderTabs();
    return;
  }
  let result;
  try {
    result = window.agnosticAIRender(els.kind.value, els.source.value, targets);
  } catch (e) {
    setStatus(`render failed: ${e.message || e}`, true);
    return;
  }
  const messages = [];
  if (result.errors && result.errors.length) {
    messages.push(result.errors
      .map((e) => `${e.target || "(input)"}: ${e.message}`)
      .join(" · "));
  }
  if (skipped.length) {
    messages.push(`Skipped for ${kind}: ${skipped.join(", ")}.`);
  }
  setStatus(messages.length ? messages.join(" ") : null, !!result.errors?.length);
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

function handleDownload() {
  const f = currentFileObject();
  if (!f) return;
  const blob = new Blob([f.content], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = f.path.split("/").pop() || "output.txt";
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
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

  const prefs = loadPrefs();
  if (prefs.kind && SAMPLES[prefs.kind]) els.kind.value = prefs.kind;
  const capabilities = window.agnosticAICapabilities()
    .sort((a, b) => a.name.localeCompare(b.name));
  buildTargetChips(capabilities, prefs.targets);
  els.source.value = SAMPLES[els.kind.value] || SAMPLES.agent;
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
    updateCapabilityState();
    savePrefs();
    scheduleRender();
  });
  els.files.addEventListener("change", () => {
    currentFile = els.files.value;
    renderTabs();
  });
  els.copy.addEventListener("click", handleCopy);
  els.download.addEventListener("click", handleDownload);

  els.app.hidden = false;
  setStatus(null);
  runRender();
}

init();
