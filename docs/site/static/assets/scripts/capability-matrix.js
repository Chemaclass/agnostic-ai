(function (root, factory) {
  "use strict";

  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.CapabilityMatrix = api;

  if (root.document) {
    api.init(root.document, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function normalizeTargets(values) {
    return Array.from(new Set((values || []).map(function (value) {
      return String(value).trim().toLowerCase();
    }).filter(Boolean))).sort();
  }

  function queryTerms(query) {
    return String(query || "").trim().toLowerCase().split(/\s+/).filter(Boolean);
  }

  function matchesTarget(target, state) {
    const selected = normalizeTargets(state.targets);
    if (selected.length > 0 && !selected.includes(target.id)) {
      return false;
    }
    const searchable = String(target.search || "").toLowerCase();
    return queryTerms(state.query).every(function (term) {
      return searchable.includes(term);
    });
  }

  function filterTargets(targets, state) {
    return targets.filter(function (target) {
      return matchesTarget(target, state);
    });
  }

  function parseURL(value) {
    const url = value instanceof URL ? value : new URL(value, "https://example.invalid/");
    return {
      targets: normalizeTargets(url.searchParams.getAll("target")),
      query: (url.searchParams.get("q") || "").trim().replace(/\s+/g, " ")
    };
  }

  function serializeURL(value, state) {
    const url = value instanceof URL ? new URL(value.href) : new URL(value, "https://example.invalid/");
    url.searchParams.delete("target");
    url.searchParams.delete("q");
    normalizeTargets(state.targets).forEach(function (target) {
      url.searchParams.append("target", target);
    });
    const query = String(state.query || "").trim().replace(/\s+/g, " ");
    if (query) {
      url.searchParams.set("q", query);
    }
    url.hash = "capability-matrix";
    return url;
  }

  function init(document, browser) {
    const browserRoot = document.querySelector("[data-capability-browser]");
    if (!browserRoot) {
      return false;
    }
    const form = browserRoot.querySelector("[data-capability-filter]");
    const rows = Array.from(browserRoot.querySelectorAll("[data-capability-target]"));
    const result = browserRoot.querySelector("[data-capability-result]");
    const notice = browserRoot.querySelector("[data-capability-notice]");
    const empty = browserRoot.querySelector("[data-capability-empty]");
    const region = browserRoot.querySelector(".capability-matrix-region");
    const active = browserRoot.querySelector("[data-active-targets]");
    const selectionCount = browserRoot.querySelector("[data-selection-count]");
    const capabilityCount = browserRoot.dataset.capabilityCount;
    if (!form || rows.length === 0 || !result || !notice || !empty || !region || !active || !selectionCount) {
      return false;
    }

    const search = form.elements.q;
    const picker = form.querySelector(".capability-target-picker");
    const checkboxes = Array.from(form.querySelectorAll('input[name="target"]'));
    const knownTargets = new Map(checkboxes.map(function (checkbox) {
      return [checkbox.value, checkbox.dataset.label];
    }));
    const targets = rows.map(function (row) {
      return {
        id: row.dataset.capabilityTarget,
        search: row.dataset.capabilitySearch,
        row: row
      };
    });

    function readForm() {
      return {
        targets: checkboxes.filter(function (checkbox) {
          return checkbox.checked;
        }).map(function (checkbox) {
          return checkbox.value;
        }),
        query: search.value
      };
    }

    function push(state) {
      const url = serializeURL(browser.location.href, state);
      browser.history.pushState(null, "", url.pathname + url.search + url.hash);
    }

    function addActiveTarget(target, state) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "capability-target-chip";
      button.textContent = knownTargets.get(target) || target;
      button.setAttribute("aria-label", "Remove " + button.textContent + " from comparison");
      button.addEventListener("click", function () {
        const next = {
          targets: normalizeTargets(state.targets).filter(function (value) {
            return value !== target;
          }),
          query: state.query
        };
        render(next);
        push(next);
      });
      active.appendChild(button);
    }

    function render(state) {
      const selectedTargets = normalizeTargets(state.targets);
      const selectedSet = new Set(selectedTargets);
      const query = String(state.query || "").trim().replace(/\s+/g, " ");
      search.value = query;
      checkboxes.forEach(function (checkbox) {
        checkbox.checked = selectedSet.has(checkbox.value);
      });

      const visible = new Set(filterTargets(targets, {
        targets: selectedTargets,
        query: query
      }));
      targets.forEach(function (target) {
        target.row.hidden = !visible.has(target);
      });
      region.hidden = visible.size === 0;
      empty.hidden = visible.size !== 0;
      result.textContent = visible.size + " of " + targets.length + " targets, " + capabilityCount + " capabilities";
      selectionCount.textContent = selectedTargets.length === 0 ? "All" : selectedTargets.length + " selected";

      active.replaceChildren();
      selectedTargets.forEach(function (target) {
        addActiveTarget(target, { targets: selectedTargets, query: query });
      });
      active.hidden = selectedTargets.length === 0;

      const unknown = selectedTargets.filter(function (target) {
        return !knownTargets.has(target);
      });
      notice.hidden = unknown.length === 0;
      notice.textContent = unknown.length === 0 ? "" : "Unknown target " + (unknown.length === 1 ? "filter" : "filters") + ": " + unknown.join(", ") + ". Remove or replace it to restore matching targets.";
    }

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      const state = readForm();
      render(state);
      push(state);
      if (picker) {
        picker.open = false;
      }
    });

    Array.from(browserRoot.querySelectorAll("[data-clear-capabilities]")).forEach(function (button) {
      button.addEventListener("click", function () {
        const state = { targets: [], query: "" };
        render(state);
        push(state);
        search.focus();
      });
    });

    browser.addEventListener("popstate", function () {
      render(parseURL(browser.location.href));
    });

    render(parseURL(browser.location.href));
    form.hidden = false;
    return true;
  }

  return {
    filterTargets: filterTargets,
    init: init,
    matchesTarget: matchesTarget,
    normalizeTargets: normalizeTargets,
    parseURL: parseURL,
    queryTerms: queryTerms,
    serializeURL: serializeURL
  };
});
