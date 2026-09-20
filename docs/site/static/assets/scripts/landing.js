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

  // The output list is a vertical tablist over the generated files. Selection
  // follows click and arrow keys, never hover, and the panes stay grid-stacked
  // so the card keeps one height.
  function initOutputSwitch(document) {
    var root = document.querySelector("[data-output-switch]");
    if (!root) {
      return false;
    }

    var tabs = Array.prototype.slice.call(root.querySelectorAll("[data-output-tab]"));
    var panels = Array.prototype.slice.call(root.querySelectorAll("[data-output-panel]"));
    if (tabs.length < 2 || tabs.length !== panels.length) {
      return false;
    }

    function select(index, moveFocus) {
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

    tabs.forEach(function (tab, index) {
      tab.addEventListener("click", function () {
        select(index, false);
      });
      tab.addEventListener("keydown", function (event) {
        var target = nextTabIndex(event.key, index, tabs.length);
        if (target < 0) {
          return;
        }
        event.preventDefault();
        select(target, true);
      });
    });
    return true;
  }

  function embedURL(videoId) {
    if (!/^[A-Za-z0-9_-]{11}$/.test(String(videoId || ""))) {
      return null;
    }
    return "https://www.youtube-nocookie.com/embed/" + videoId + "?autoplay=1&rel=0";
  }

  function initVideoFacade(document) {
    var facade = document.querySelector("[data-video-facade]");
    var link = facade && facade.querySelector("[data-video-play]");
    if (!link) {
      return false;
    }

    link.addEventListener("click", function (event) {
      var src = embedURL(facade.dataset.videoId);
      if (!src) {
        return;
      }
      event.preventDefault();
      var iframe = document.createElement("iframe");
      iframe.src = src;
      iframe.title = facade.dataset.videoTitle || "";
      iframe.setAttribute("allow", "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share");
      iframe.setAttribute("allowfullscreen", "");
      iframe.setAttribute("referrerpolicy", "strict-origin-when-cross-origin");
      link.replaceWith(iframe);
      iframe.focus();
    });
    return true;
  }

  function init(document, browser) {
    initHeroInstaller(document, browser);
    initOutputSwitch(document);
    initReveal(document, browser);
    initVideoFacade(document);
  }

  return {
    detectOS: detectOS,
    embedURL: embedURL,
    init: init,
    initHeroInstaller: initHeroInstaller,
    initOutputSwitch: initOutputSwitch,
    initVideoFacade: initVideoFacade,
    nextTabIndex: nextTabIndex
  };
});
