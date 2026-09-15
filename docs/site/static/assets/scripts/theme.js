(function () {
  "use strict";

  var root = document.documentElement;
  var storageKey = "aai-theme";

  try {
    var savedTheme = localStorage.getItem(storageKey);
    if (savedTheme === "light" || savedTheme === "dark") {
      root.setAttribute("data-theme", savedTheme);
    }
  } catch (_) {
    // Storage can be unavailable in private browsing. The system theme still works.
  }

  function currentTheme() {
    var explicitTheme = root.getAttribute("data-theme");
    if (explicitTheme) {
      return explicitTheme;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  }

  function updateButton(button) {
    var nextTheme = currentTheme() === "dark" ? "light" : "dark";
    button.setAttribute("aria-label", "Use " + nextTheme + " theme");
  }

  document.addEventListener("DOMContentLoaded", function () {
    var button = document.querySelector(".theme-toggle");
    if (!button) {
      return;
    }

    updateButton(button);
    button.addEventListener("click", function () {
      var nextTheme = currentTheme() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", nextTheme);
      updateButton(button);
      try {
        localStorage.setItem(storageKey, nextTheme);
      } catch (_) {
        // Theme changes still apply for this page when storage is unavailable.
      }
    });
  });
})();
