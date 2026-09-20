"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { detectOS, init, initOutputSwitch, initReveal, nextTabIndex } = require("./landing.js");

test("detects supported desktop operating systems", function () {
  assert.equal(detectOS("macOS", "", 0), "macos");
  assert.equal(detectOS("Win32", "", 0), "windows");
  assert.equal(detectOS("Linux x86_64", "", 0), "linux");
});

test("uses the user agent when platform data is unavailable", function () {
  assert.equal(detectOS("", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)", 0), "windows");
  assert.equal(detectOS("", "Mozilla/5.0 (X11; Linux x86_64)", 0), "linux");
});

test("does not recommend desktop installers on mobile or ChromeOS", function () {
  assert.equal(detectOS("MacIntel", "Mozilla/5.0 (iPad)", 5), "other");
  assert.equal(detectOS("Linux armv8l", "Mozilla/5.0 (Linux; Android 15)", 5), "other");
  assert.equal(detectOS("Linux x86_64", "Mozilla/5.0 (X11; CrOS x86_64)", 0), "other");
});

test("leaves unknown platforms on the neutral fallback", function () {
  assert.equal(detectOS("", "", 0), "other");
  assert.equal(detectOS("FreeBSD amd64", "Mozilla/5.0", 0), "other");
});

test("moves the output tablist selection with the arrow keys, wrapping at both ends", function () {
  assert.equal(nextTabIndex("ArrowDown", 0, 5), 1);
  assert.equal(nextTabIndex("ArrowRight", 4, 5), 0);
  assert.equal(nextTabIndex("ArrowUp", 0, 5), 4);
  assert.equal(nextTabIndex("ArrowLeft", 3, 5), 2);
});

test("jumps the output tablist to the first and last file", function () {
  assert.equal(nextTabIndex("Home", 3, 5), 0);
  assert.equal(nextTabIndex("End", 1, 5), 4);
});

test("leaves keys the output tablist does not own to the browser", function () {
  assert.equal(nextTabIndex("Tab", 2, 5), -1);
  assert.equal(nextTabIndex("a", 2, 5), -1);
  assert.equal(nextTabIndex("ArrowDown", 0, 0), -1);
});

// A DOM small enough to read in one sitting. landing.js only ever asks for an
// attribute selector or a class selector, so those are the only two forms this
// understands.
function makeElement(attributes, classes) {
  const node = {
    attributes: Object.assign({}, attributes),
    classes: new Set(classes || []),
    listeners: {},
    hidden: false,
    tabIndex: -1,
    focusCount: 0
  };
  node.setAttribute = function (name, value) {
    node.attributes[name] = String(value);
  };
  node.getAttribute = function (name) {
    return Object.prototype.hasOwnProperty.call(node.attributes, name) ? node.attributes[name] : null;
  };
  node.hasAttribute = function (name) {
    return Object.prototype.hasOwnProperty.call(node.attributes, name);
  };
  node.removeAttribute = function (name) {
    delete node.attributes[name];
  };
  node.addEventListener = function (type, handler) {
    node.listeners[type] = node.listeners[type] || [];
    node.listeners[type].push(handler);
  };
  node.dispatch = function (type, event) {
    const handlers = node.listeners[type] || [];
    handlers.forEach(function (handler) {
      handler(Object.assign({ preventDefault: function () {} }, event));
    });
    return handlers.length;
  };
  node.focus = function () {
    node.focusCount += 1;
  };
  node.classList = {
    add: function (name) { node.classes.add(name); },
    remove: function (name) { node.classes.delete(name); },
    contains: function (name) { return node.classes.has(name); }
  };
  return node;
}

function matchesSelector(node, selector) {
  if (selector.charAt(0) === "[") {
    return node.hasAttribute(selector.slice(1, -1));
  }
  return node.classes.has(selector.slice(1));
}

function scope(nodes) {
  return {
    querySelector: function (selector) {
      return nodes.filter(function (node) { return matchesSelector(node, selector); })[0] || null;
    },
    querySelectorAll: function (selector) {
      return nodes.filter(function (node) { return matchesSelector(node, selector); });
    }
  };
}

// The generated files as the template renders them: the first tab selected,
// the rest hidden, and the whole list inert until a script proves it can do
// something with a press.
function makeOutputSwitch(count) {
  const list = makeElement({ "data-output-tablist": "", inert: "" });
  const tabs = [];
  const panels = [];
  for (let index = 0; index < count; index += 1) {
    const selected = index === 0;
    const tab = makeElement({ "data-output-tab": "", "aria-selected": selected ? "true" : "false" });
    tab.tabIndex = selected ? 0 : -1;
    const panel = makeElement({ "data-output-panel": "" });
    panel.hidden = !selected;
    tabs.push(tab);
    panels.push(panel);
  }
  const root = makeElement({ "data-output-switch": "" });
  Object.assign(root, scope([list].concat(tabs, panels)));
  return { list: list, root: root, tabs: tabs, panels: panels };
}

test("the output tablist stays inert until the script has wired every tab", function () {
  const fixture = makeOutputSwitch(5);
  assert.equal(fixture.list.hasAttribute("inert"), true);

  assert.equal(initOutputSwitch(scope([fixture.root])), true);
  assert.equal(fixture.list.hasAttribute("inert"), false);

  fixture.tabs[2].dispatch("click");
  assert.equal(fixture.tabs[2].getAttribute("aria-selected"), "true");
  assert.equal(fixture.tabs[0].getAttribute("aria-selected"), "false");
  assert.equal(fixture.panels[2].hidden, false);
  assert.equal(fixture.panels[0].hidden, true);
});

test("an output tablist the script cannot wire keeps its inert resting state", function () {
  const fixture = makeOutputSwitch(1);
  assert.equal(initOutputSwitch(scope([fixture.root])), false);
  assert.equal(fixture.list.hasAttribute("inert"), true);
  assert.equal(fixture.tabs[0].dispatch("click"), 0);
});

test("a page without the hero diagram is not an error", function () {
  assert.equal(initOutputSwitch(scope([])), false);
});

test("one feature throwing during init does not leave the tabs dead", function () {
  const fixture = makeOutputSwitch(5);
  const heroInstaller = makeElement({ "data-hero-installer": "" });
  const errors = [];
  const browser = {
    console: { error: function (error) { errors.push(error); } },
    get navigator() {
      throw new Error("navigator is unavailable");
    }
  };

  init(scope([heroInstaller, fixture.root]), browser);

  assert.equal(errors.length, 1);
  assert.equal(fixture.list.hasAttribute("inert"), false);
  fixture.tabs[3].dispatch("click");
  assert.equal(fixture.tabs[3].getAttribute("aria-selected"), "true");
});

function makeRevealHarness(reduceMotion) {
  const observers = [];
  function FakeObserver(callback) {
    this.callback = callback;
    this.observed = [];
    this.unobserved = [];
    observers.push(this);
  }
  FakeObserver.prototype.observe = function (node) { this.observed.push(node); };
  FakeObserver.prototype.unobserve = function (node) { this.unobserved.push(node); };
  return {
    observers: observers,
    browser: {
      IntersectionObserver: FakeObserver,
      matchMedia: function (query) {
        return { matches: reduceMotion && query === "(prefers-reduced-motion: reduce)" };
      }
    }
  };
}

test("the scroll reveal moves a group from pending to revealed exactly once", function () {
  const group = makeElement({}, ["reveal-group"]);
  const harness = makeRevealHarness(false);

  initReveal(scope([group]), harness.browser);
  assert.equal(group.classList.contains("is-reveal-pending"), true);
  assert.deepEqual(harness.observers[0].observed, [group]);

  harness.observers[0].callback([{ isIntersecting: false, target: group }]);
  assert.equal(group.classList.contains("is-revealed"), false);

  harness.observers[0].callback([{ isIntersecting: true, target: group }]);
  assert.equal(group.classList.contains("is-reveal-pending"), false);
  assert.equal(group.classList.contains("is-revealed"), true);
  assert.deepEqual(harness.observers[0].unobserved, [group]);
});

// The class the reveal adds is what hides a group, so a visitor asking for
// reduced motion must never receive it: no observer, and every group left at
// the rest state the animation would have finished on.
test("asking for reduced motion leaves every group at its finished rest state", function () {
  const group = makeElement({}, ["reveal-group"]);
  const harness = makeRevealHarness(true);

  initReveal(scope([group]), harness.browser);

  assert.equal(harness.observers.length, 0);
  assert.equal(group.classList.contains("is-reveal-pending"), false);
  assert.equal(group.classList.contains("is-revealed"), false);
});

test("a browser without IntersectionObserver never hides a group", function () {
  const group = makeElement({}, ["reveal-group"]);

  initReveal(scope([group]), { matchMedia: function () { return { matches: false }; } });

  assert.equal(group.classList.contains("is-reveal-pending"), false);
});
