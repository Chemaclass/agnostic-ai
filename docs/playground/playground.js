"use strict";

const SAMPLE = `---
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
`;

const DEFAULT_TARGETS = ["claude", "codex", "cursor", "gemini"];

const $ = (id) => document.getElementById(id);
const status = $("status");
const app = $("app");
const source = $("source");
const kind = $("kind");
const targetsBox = $("targets");
const tabsBox = $("tabs");
const content = $("content");

let renderResults = [];
let currentTarget = null;

function setStatus(msg, isError) {
  if (msg == null) {
    status.hidden = true;
    return;
  }
  status.hidden = false;
  status.textContent = msg;
  status.classList.toggle("error", !!isError);
}

function buildTargetCheckboxes(allTargets) {
  targetsBox.querySelectorAll("label").forEach((n) => n.remove());
  allTargets.forEach((name) => {
    const id = `target-${name}`;
    const label = document.createElement("label");
    label.htmlFor = id;
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.id = id;
    cb.value = name;
    cb.checked = DEFAULT_TARGETS.includes(name);
    cb.addEventListener("change", scheduleRender);
    label.append(cb, document.createTextNode(name));
    targetsBox.append(label);
  });
}

function selectedTargets() {
  return Array.from(
    targetsBox.querySelectorAll('input[type="checkbox"]:checked'),
  ).map((cb) => cb.value);
}

function renderTabs() {
  tabsBox.innerHTML = "";
  const byTarget = new Map();
  renderResults.forEach((f) => {
    if (!byTarget.has(f.target)) byTarget.set(f.target, []);
    byTarget.get(f.target).push(f);
  });
  if (byTarget.size === 0) {
    content.textContent = "";
    return;
  }
  const targets = Array.from(byTarget.keys()).sort();
  if (!targets.includes(currentTarget)) currentTarget = targets[0];
  targets.forEach((t) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.role = "tab";
    btn.textContent = t;
    btn.setAttribute("aria-selected", t === currentTarget);
    btn.addEventListener("click", () => {
      currentTarget = t;
      renderTabs();
    });
    tabsBox.append(btn);
  });
  const files = byTarget.get(currentTarget) || [];
  content.textContent = files
    .map((f) => `// ${f.path}\n${f.content}`)
    .join("\n\n");
}

let pending = 0;
function scheduleRender() {
  const seq = ++pending;
  setTimeout(() => {
    if (seq !== pending) return;
    runRender();
  }, 80);
}

function runRender() {
  const targets = selectedTargets();
  if (targets.length === 0) {
    renderResults = [];
    setStatus("Pick at least one target.");
    renderTabs();
    return;
  }
  let result;
  try {
    result = window.agnosticAIRender(kind.value, source.value, targets);
  } catch (e) {
    setStatus(`render failed: ${e.message || e}`, true);
    return;
  }
  if (result.errors && result.errors.length) {
    const lines = result.errors
      .map((e) => `${e.target || "(input)"}: ${e.message}`)
      .join(" · ");
    setStatus(lines, true);
  } else {
    setStatus(null);
  }
  renderResults = result.files || [];
  renderTabs();
}

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

  const allTargets = window.agnosticAITargets().sort();
  buildTargetCheckboxes(allTargets);
  source.value = SAMPLE;
  source.addEventListener("input", scheduleRender);
  kind.addEventListener("change", scheduleRender);
  app.hidden = false;
  setStatus(null);
  runRender();
}

init();
