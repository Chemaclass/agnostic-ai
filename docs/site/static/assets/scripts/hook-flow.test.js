"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { initFlow } = require("./hook-flow.js");

function node(attributes, text) {
  const listeners = new Map();
  const classes = new Set();
  const values = Object.assign({}, attributes);
  return {
    textContent: text || "",
    hidden: false,
    disabled: false,
    getAttribute: name => values[name] === undefined ? null : values[name],
    hasAttribute: name => values[name] !== undefined,
    setAttribute: (name, value) => { values[name] = String(value); },
    removeAttribute: name => { delete values[name]; },
    classList: { toggle: (name, active) => active ? classes.add(name) : classes.delete(name) },
    addEventListener: (name, callback) => listeners.set(name, callback),
    removeEventListener: name => listeners.delete(name),
    dispatch: name => { if (listeners.has(name)) listeners.get(name)(); }
  };
}

function fixture(reduced) {
  const html = fs.readFileSync(path.join(__dirname, "../../../templates/components/hook_flow.html"), "utf8");
  const stages = Array.from(html.matchAll(/<li data-hook-stage="([^"]+)"/g), match => node({ "data-hook-stage": match[1] }));
  const example = Array.from(html.matchAll(/<p data-hook-phase="([^"]+)"([^>]*)>([^<]+)<\/p>/g), match => node(Object.assign({ "data-hook-phase": match[1] }, match[2].includes("data-hook-edit") ? { "data-hook-edit": "" } : {}), match[3]));
  const controls = node();
  controls.hidden = true;
  const play = node();
  const next = node();
  const reset = node();
  const status = node({}, "Follow the example.");
  const note = node();
  const elements = { "[data-hook-controls]": controls, "[data-hook-play]": play, "[data-hook-next]": next, "[data-hook-reset]": reset, "[data-hook-status]": status, "[data-hook-reduced]": note };
  const flow = node();
  flow.querySelector = selector => elements[selector];
  flow.querySelectorAll = selector => selector === "[data-hook-stage]" ? stages : example;
  const document = node();
  document.hidden = false;
  const motion = node();
  motion.matches = !!reduced;
  const timers = new Map();
  let timerId = 0;
  const view = Object.assign(node(), {
    matchMedia: () => motion,
    setTimeout: callback => { timers.set(++timerId, callback); return timerId; },
    clearTimeout: id => timers.delete(id)
  });
  const api = initFlow(flow, document, view);
  const tick = () => {
    const entry = timers.entries().next().value;
    assert.ok(entry, "a playback step is scheduled");
    timers.delete(entry[0]);
    entry[1]();
  };
  return { flow, stages, example, controls, play, next, reset, status, note, motion, timers, document, view, api, tick };
}

test("plays one finite example with repeated tools and a conditional edit event", () => {
  const f = fixture();
  assert.equal(f.controls.hidden, false);
  assert.equal(f.timers.size, 0);
  assert.equal(f.stages.some(stage => stage.hasAttribute("aria-current")), false);
  f.play.dispatch("click");
  assert.equal(f.stages[0].getAttribute("aria-current"), "step");
  f.tick(); f.tick(); f.tick();
  assert.equal(f.stages[3].getAttribute("aria-current"), "step");
  assert.equal(f.flow.getAttribute("data-hook-edit"), "false");
  assert.match(f.status.textContent, /no file was edited/);
  f.tick(); f.tick();
  assert.equal(f.flow.getAttribute("data-hook-edit"), "true");
  assert.match(f.status.textContent, /Their order depends on the coding tool/);
  f.tick(); f.tick();
  assert.equal(f.stages[5].getAttribute("aria-current"), "step");
  assert.equal(f.timers.size, 0);
  assert.equal(f.play.textContent, "Replay example");
  assert.equal(f.play.getAttribute("aria-pressed"), "false");
  assert.equal(f.next.disabled, true);
});

test("stops playback when hidden or reduced motion is enabled and keeps manual steps usable", () => {
  const f = fixture();
  f.play.dispatch("click");
  f.document.hidden = true;
  f.document.dispatch("visibilitychange");
  assert.equal(f.timers.size, 0);
  f.document.hidden = false;
  f.play.dispatch("click");
  assert.equal(f.timers.size, 1);
  f.motion.matches = true;
  f.motion.dispatch("change");
  assert.equal(f.timers.size, 0);
  assert.equal(f.play.hidden, true);
  assert.equal(f.note.hidden, false);
  f.next.dispatch("click");
  assert.equal(f.stages[1].getAttribute("aria-current"), "step");
  assert.equal(f.timers.size, 0);
  f.reset.dispatch("click");
  assert.equal(f.stages.some(stage => stage.hasAttribute("aria-current")), false);
  assert.equal(f.status.textContent, "Follow the example.");
});

test("handles pause, replay, reset and page cleanup without duplicate playback listeners", () => {
  const f = fixture();
  assert.equal(initFlow(f.flow, f.document, f.view), null);
  f.play.dispatch("click");
  f.play.dispatch("click");
  assert.equal(f.timers.size, 0);
  f.play.dispatch("click");
  f.view.dispatch("pagehide");
  assert.equal(f.timers.size, 0);
  f.play.dispatch("click");
  f.api.destroy();
  assert.equal(f.timers.size, 0);
  f.play.dispatch("click");
  assert.equal(f.timers.size, 0);
});
