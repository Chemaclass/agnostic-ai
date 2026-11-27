"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const { detectOS, init, initCompanionDownloads, initHeroInstaller, initInstallPicker, initOutputSwitch, initSourceTree, nextTabIndex } = require("./landing.js");

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

test("companion downloads use the available stable layout", async function () {
  const releases = "https://github.com/Chemaclass/agnostic-ai/releases";
  const fallback = releases + "/latest";
  const api = "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest";
  const current = "agnostic-companion_darwin_arm64.dmg";
  const legacy = "agnostic-ai-companion_darwin_arm64.dmg";
  const button = makeElement({ href: fallback, "data-companion-asset": current });
  const downloads = makeElement({
    "data-companion-release-root": releases,
    "data-companion-release-api": api
  });
  Object.assign(downloads, scope([button]));
  const document = scope([downloads]);
  const browser = {
    fetch: async function (url, options) {
      assert.equal(url, api);
      assert.equal(options.credentials, "omit");
      return { ok: true, json: async function () { return release; } };
    }
  };
  let release = { tag_name: "v0.99.0", assets: [{ name: legacy, state: "uploaded" }] };

  await initCompanionDownloads(document, browser);
  assert.equal(button.getAttribute("href"), releases + "/download/v0.99.0/" + legacy);
  release = { tag_name: "v0.100.0", assets: [
    { name: legacy, state: "uploaded" },
    { name: current, state: "uploaded" }
  ] };
  await initCompanionDownloads(document, browser);
  assert.equal(button.getAttribute("href"), releases + "/download/v0.100.0/" + current);
});

test("companion downloads keep the release page when assets are unavailable", async function () {
  const releases = "https://github.com/Chemaclass/agnostic-ai/releases";
  const button = makeElement({
    href: releases + "/latest",
    "data-companion-asset": "agnostic-companion_darwin_amd64.dmg"
  });
  const downloads = makeElement({
    "data-companion-release-root": releases,
    "data-companion-release-api": "https://api.github.com/repos/Chemaclass/agnostic-ai/releases/latest"
  });
  Object.assign(downloads, scope([button]));

  await initCompanionDownloads(scope([downloads]), { fetch: async function () { throw new Error("offline"); } });
  assert.equal(button.getAttribute("href"), releases + "/latest");
});

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

test("every output switch on the page is wired, not only the first", function () {
  const first = makeOutputSwitch(3);
  const second = makeOutputSwitch(4);

  assert.equal(initOutputSwitch(scope([first.root, second.root])), true);

  assert.equal(first.list.hasAttribute("inert"), false);
  assert.equal(second.list.hasAttribute("inert"), false);
  second.tabs[3].dispatch("click");
  assert.equal(second.panels[3].hidden, false);
  assert.equal(first.panels[0].hidden, false);
});

test("a page without output switches is not an error", function () {
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

test("the source tree drops inert and switches entries on click and arrow keys", function () {
  const list = makeElement({ "data-source-tablist": "", inert: "" });
  const tabs = [];
  const panels = [];
  for (let index = 0; index < 3; index += 1) {
    tabs.push(makeElement({ "data-source-tab": "", "aria-selected": index === 0 ? "true" : "false" }));
    const panel = makeElement({ "data-source-panel": "" });
    panel.hidden = index !== 0;
    panels.push(panel);
  }
  const root = makeElement({ "data-source-tree": "" });
  Object.assign(root, scope([list].concat(tabs, panels)));

  assert.equal(initSourceTree(scope([root])), true);
  assert.equal(list.hasAttribute("inert"), false);

  tabs[1].dispatch("click");
  assert.equal(panels[1].hidden, false);
  assert.equal(panels[0].hidden, true);

  tabs[1].dispatch("keydown", { key: "ArrowDown" });
  assert.equal(tabs[2].getAttribute("aria-selected"), "true");
  assert.equal(tabs[2].focusCount, 1);
});

test("a page without the source tree is not an error", function () {
  assert.equal(initSourceTree(scope([])), false);
});

// The installer picker as the template renders it: the tab strip hidden, the
// first tab selected, and every panel visible so a reader without the script
// sees each command.
function makeInstallPicker(oses) {
  const list = makeElement({ "data-install-tablist": "" });
  list.hidden = true;
  const tabs = [];
  const panels = [];
  oses.forEach(function (os, index) {
    const attributes = { "data-install-tab": "", "aria-selected": index === 0 ? "true" : "false" };
    if (os) {
      attributes["data-installer-os"] = os;
    }
    const tab = makeElement(attributes);
    tab.tabIndex = index === 0 ? 0 : -1;
    tabs.push(tab);
    panels.push(makeElement({ "data-install-panel": "" }));
  });
  const root = makeElement({ "data-install-picker": "" });
  Object.assign(root, scope([list].concat(tabs, panels)));
  return { list: list, root: root, tabs: tabs, panels: panels };
}

function browserOn(platform) {
  return { navigator: { platform: platform, userAgent: "", maxTouchPoints: 0 } };
}

test("the installer picker opens the tab for the reader's OS", function () {
  const fixture = makeInstallPicker(["macos", "linux", "windows", ""]);

  assert.equal(initInstallPicker(scope([fixture.root]), browserOn("Win32")), true);

  assert.equal(fixture.list.hidden, false);
  assert.equal(fixture.root.classList.contains("is-tabbed"), true);
  assert.equal(fixture.tabs[2].getAttribute("aria-selected"), "true");
  assert.equal(fixture.tabs[0].getAttribute("aria-selected"), "false");
  assert.deepEqual(fixture.panels.map(function (panel) { return panel.hidden; }), [true, true, false, true]);
});

test("the installer picker keeps the first tab for an OS with no tab of its own", function () {
  const fixture = makeInstallPicker(["macos", "linux", "windows", ""]);

  initInstallPicker(scope([fixture.root]), browserOn("FreeBSD amd64"));

  assert.equal(fixture.tabs[0].getAttribute("aria-selected"), "true");
  assert.equal(fixture.panels[0].hidden, false);
  assert.equal(fixture.panels[3].hidden, true);
});

test("the installer picker switches panels on click and arrow keys", function () {
  const fixture = makeInstallPicker(["macos", "linux", "windows", ""]);
  initInstallPicker(scope([fixture.root]), browserOn("macOS"));

  fixture.tabs[3].dispatch("click");
  assert.equal(fixture.panels[3].hidden, false);
  assert.equal(fixture.panels[0].hidden, true);

  fixture.tabs[3].dispatch("keydown", { key: "ArrowRight" });
  assert.equal(fixture.tabs[0].getAttribute("aria-selected"), "true");
  assert.equal(fixture.tabs[0].focusCount, 1);
});

test("an installer picker the script cannot wire keeps every panel visible", function () {
  const fixture = makeInstallPicker(["macos"]);

  assert.equal(initInstallPicker(scope([fixture.root]), browserOn("macOS")), false);

  assert.equal(fixture.list.hidden, true);
  assert.equal(fixture.panels[0].hidden, false);
  assert.equal(initInstallPicker(scope([]), browserOn("macOS")), false);
});

// The hero install block as the template renders it: the script installer,
// its note, and one hidden option per OS that has a recommended installer.
function makeHeroInstaller() {
  const prompt = { textContent: "$" };
  const code = { textContent: "curl script" };
  const copy = makeElement({ "aria-label": "Copy Script install command" });
  const command = {
    querySelector: function (selector) {
      return { "span[aria-hidden]": prompt, code: code, "[data-copy]": copy }[selector] || null;
    }
  };
  const label = { textContent: "For macOS and Linux." };
  const others = { textContent: "Windows, Homebrew or npm" };
  const option = {
    dataset: {
      installerLabel: "Detected macOS.",
      installerOthers: "Linux, Windows or npm",
      installerPrompt: "$",
      installerCommand: "brew install agnostic-ai",
      installerName: "Homebrew"
    }
  };
  const root = {
    dataset: {},
    querySelector: function (selector) {
      return {
        '[data-installer-os="macos"]': option,
        "[data-hero-installer-label]": label,
        "[data-hero-installer-others]": others,
        ".command": command
      }[selector] || null;
    }
  };
  const document = {
    querySelector: function (selector) {
      return selector === "[data-hero-installer]" ? root : null;
    }
  };
  return { document: document, root: root, code: code, copy: copy, label: label, others: others };
}

test("the hero shows the installer for the reader's OS and names the others", function () {
  const hero = makeHeroInstaller();

  assert.equal(initHeroInstaller(hero.document, browserOn("macOS")), true);

  assert.equal(hero.code.textContent, "brew install agnostic-ai");
  assert.equal(hero.label.textContent, "Detected macOS.");
  assert.equal(hero.others.textContent, "Linux, Windows or npm");
  assert.equal(hero.copy.getAttribute("aria-label"), "Copy Homebrew install command");
  assert.equal(hero.root.dataset.detectedOs, "macos");
});

test("the hero keeps the default installer for an OS without a recommendation", function () {
  const hero = makeHeroInstaller();

  assert.equal(initHeroInstaller(hero.document, browserOn("FreeBSD amd64")), false);

  assert.equal(hero.code.textContent, "curl script");
  assert.equal(hero.label.textContent, "For macOS and Linux.");
  assert.equal(hero.others.textContent, "Windows, Homebrew or npm");
  assert.equal(hero.root.dataset.detectedOs, "other");
});
