(function (root, factory) {
  "use strict";

  var api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.AgnosticAILanding = api;

  if (root.document) {
    api.init(root.document, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function detectOS(platform, userAgent, maxTouchPoints) {
    var value = (String(platform || "") + " " + String(userAgent || "")).toLowerCase();
    if (/android|iphone|ipad|ipod|cros/.test(value)) {
      return "other";
    }
    if (/windows|win32|win64/.test(value)) {
      return "windows";
    }
    if (/mac|darwin/.test(value)) {
      return Number(maxTouchPoints || 0) > 1 ? "other" : "macos";
    }
    if (/linux|x11/.test(value)) {
      return "linux";
    }
    return "other";
  }

  function browserOS(browser) {
    var navigator = browser.navigator || {};
    var userAgentData = navigator.userAgentData || {};
    return detectOS(userAgentData.platform || navigator.platform, navigator.userAgent, navigator.maxTouchPoints);
  }

  function initHeroInstaller(document, browser) {
    var root = document.querySelector("[data-hero-installer]");
    if (!root) {
      return false;
    }

    var os = browserOS(browser);
    var option = root.querySelector('[data-installer-os="' + os + '"]');
    if (!option) {
      root.dataset.detectedOs = "other";
      return false;
    }

    var label = root.querySelector("[data-hero-installer-label]");
    var command = root.querySelector(".command");
    var prompt = command && command.querySelector("span[aria-hidden]");
    var code = command && command.querySelector("code");
    var copy = command && command.querySelector("[data-copy]");
    if (!label || !prompt || !code || !copy) {
      return false;
    }

    label.textContent = option.dataset.installerLabel;
    prompt.textContent = option.dataset.installerPrompt;
    code.textContent = option.dataset.installerCommand;
    copy.setAttribute("aria-label", "Copy " + option.dataset.installerName + " install command");
    root.dataset.detectedOs = os;
    return true;
  }

  function initReveal(document, browser) {
    var reduceMotion = typeof browser.matchMedia === "function" && browser.matchMedia("(prefers-reduced-motion: reduce)").matches;
    var revealGroups = document.querySelectorAll(".reveal-group");
    if (reduceMotion || !("IntersectionObserver" in browser)) {
      return;
    }
    var observer = new browser.IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (!entry.isIntersecting) {
          return;
        }
        entry.target.classList.remove("is-reveal-pending");
        entry.target.classList.add("is-revealed");
        observer.unobserve(entry.target);
      });
    }, { threshold: 0.08 });

    Array.prototype.forEach.call(revealGroups, function (group) {
      group.classList.add("is-reveal-pending");
      observer.observe(group);
    });
  }

  // Arrow keys wrap, Home and End jump to the ends. Returns the index the
  // key moves to, or -1 when the key is not one this list handles.
  function nextTabIndex(key, current, total) {
    if (total < 1) {
      return -1;
    }
    if (key === "ArrowDown" || key === "ArrowRight") {
      return (current + 1) % total;
    }
    if (key === "ArrowUp" || key === "ArrowLeft") {
      return (current - 1 + total) % total;
    }
    if (key === "Home") {
      return 0;
    }
    if (key === "End") {
      return total - 1;
    }
    return -1;
  }

  function selectTab(tabs, panels, index, moveFocus) {
    tabs.forEach(function (tab, position) {
      var active = position === index;
      tab.setAttribute("aria-selected", active ? "true" : "false");
      tab.tabIndex = active ? 0 : -1;
      panels[position].hidden = !active;
    });
    if (moveFocus) {
      tabs[index].focus();
    }
  }

  function wireTabs(tabs, panels) {
    tabs.forEach(function (tab, index) {
      tab.addEventListener("click", function () {
        selectTab(tabs, panels, index, false);
      });
      tab.addEventListener("keydown", function (event) {
        var target = nextTabIndex(event.key, index, tabs.length);
        if (target < 0) {
          return;
        }
        event.preventDefault();
        selectTab(tabs, panels, target, true);
      });
    });
  }

  // A tablist that ships inert beside server-rendered panes. Selection
  // follows click and arrow keys, never hover, and the panes stay grid-stacked
  // so the card keeps one height.
  function initInertTabs(document, names) {
    var root = document.querySelector("[" + names.root + "]");
    if (!root) {
      return false;
    }

    var list = root.querySelector("[" + names.list + "]");
    var tabs = Array.prototype.slice.call(root.querySelectorAll("[" + names.tab + "]"));
    var panels = Array.prototype.slice.call(root.querySelectorAll("[" + names.panel + "]"));
    if (!list || tabs.length < 2 || tabs.length !== panels.length) {
      return false;
    }

    wireTabs(tabs, panels);

    // The template ships the list inert so a reader without this script is
    // never offered buttons that cannot be pressed. Every tab is wired by the
    // time we get here, so the offer is now real. Nothing above this line may
    // fail without leaving the list inert.
    list.removeAttribute("inert");
    return true;
  }

  // The hero diagram's generated files.
  function initOutputSwitch(document) {
    return initInertTabs(document, {
      root: "data-output-switch",
      list: "data-output-tablist",
      tab: "data-output-tab",
      panel: "data-output-panel"
    });
  }

  // The landing's `.agnostic-ai/` listing.
  function initSourceTree(document) {
    return initInertTabs(document, {
      root: "data-source-tree",
      list: "data-source-tablist",
      tab: "data-source-tab",
      panel: "data-source-panel"
    });
  }

  // The installation page's installer tabs. The template ships the tab strip
  // hidden and every panel visible, so a reader without this script still
  // sees each command under its own title. The tab for the reader's OS opens
  // first; an OS with no tab keeps the template's first one.
  function initInstallPicker(document, browser) {
    var root = document.querySelector("[data-install-picker]");
    if (!root) {
      return false;
    }

    var list = root.querySelector("[data-install-tablist]");
    var tabs = Array.prototype.slice.call(root.querySelectorAll("[data-install-tab]"));
    var panels = Array.prototype.slice.call(root.querySelectorAll("[data-install-panel]"));
    if (!list || tabs.length < 2 || tabs.length !== panels.length) {
      return false;
    }

    var os = browserOS(browser);
    var detected = 0;
    tabs.forEach(function (tab, index) {
      if (tab.getAttribute("data-installer-os") === os) {
        detected = index;
      }
    });

    wireTabs(tabs, panels);
    selectTab(tabs, panels, detected, false);
    list.hidden = false;
    root.classList.add("is-tabbed");
    return true;
  }

  // One broken feature must not take the others with it. A throw here is a
  // bug worth seeing in the console, not a reason to leave the rest of the
  // page dead.
  function guard(browser, step) {
    try {
      return step();
    } catch (error) {
      if (browser.console && typeof browser.console.error === "function") {
        browser.console.error(error);
      }
      return false;
    }
  }

  function init(document, browser) {
    guard(browser, function () {
      return initHeroInstaller(document, browser);
    });
    guard(browser, function () {
      return initOutputSwitch(document);
    });
    guard(browser, function () {
      return initSourceTree(document);
    });
    guard(browser, function () {
      return initInstallPicker(document, browser);
    });
    guard(browser, function () {
      return initReveal(document, browser);
    });
  }

  return {
    detectOS: detectOS,
    init: init,
    initHeroInstaller: initHeroInstaller,
    initInstallPicker: initInstallPicker,
    initOutputSwitch: initOutputSwitch,
    initReveal: initReveal,
    initSourceTree: initSourceTree,
    nextTabIndex: nextTabIndex
  };
});
