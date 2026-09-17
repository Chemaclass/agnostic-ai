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

  function initHeroInstaller(document, browser) {
    var root = document.querySelector("[data-hero-installer]");
    if (!root) {
      return false;
    }

    var navigator = browser.navigator || {};
    var userAgentData = navigator.userAgentData || {};
    var os = detectOS(userAgentData.platform || navigator.platform, navigator.userAgent, navigator.maxTouchPoints);
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

  function init(document, browser) {
    initHeroInstaller(document, browser);
    initReveal(document, browser);
  }

  return {
    detectOS: detectOS,
    init: init,
    initHeroInstaller: initHeroInstaller
  };
});
