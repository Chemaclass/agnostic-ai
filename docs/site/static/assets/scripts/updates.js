(function (root, factory) {
  "use strict";

  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.UpdatesFilter = api;

  if (root.document) {
    api.init(root.document, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  function normalizeTargets(values) {
    return Array.from(new Set(values.map(function (value) {
      return String(value).trim().toLowerCase();
    }).filter(Boolean))).sort();
  }

  function queryTerms(query) {
    return String(query || "").trim().toLowerCase().split(/\s+/).filter(Boolean);
  }

  function matchesEdition(edition, state) {
    const selectedTargets = normalizeTargets(state.targets || []);
    const editionTargets = normalizeTargets(edition.targets || []);
    const targetMatch = selectedTargets.length === 0 || selectedTargets.some(function (target) {
      return editionTargets.includes(target);
    });
    if (!targetMatch) {
      return false;
    }

    const searchable = String(edition.search || "").toLowerCase();
    return queryTerms(state.query).every(function (term) {
      return searchable.includes(term);
    });
  }

  function filterEditions(editions, state) {
    return editions.filter(function (edition) {
      return matchesEdition(edition, state);
    });
  }

  function parseURL(value) {
    const url = value instanceof URL ? value : new URL(value, "https://example.invalid/");
    return {
      targets: normalizeTargets(url.searchParams.getAll("target")),
      query: (url.searchParams.get("q") || "").trim()
    };
  }

  function serializeURL(value, state) {
    const url = value instanceof URL ? new URL(value.href) : new URL(value, "https://example.invalid/");
    url.searchParams.delete("target");
    url.searchParams.delete("q");
    normalizeTargets(state.targets || []).forEach(function (target) {
      url.searchParams.append("target", target);
    });
    const query = String(state.query || "").trim().replace(/\s+/g, " ");
    if (query) {
      url.searchParams.set("q", query);
    }
    url.hash = "archive-title";
    return url;
  }

  function init(document, browser) {
    const form = document.querySelector("[data-updates-filter]");
    const rows = Array.from(document.querySelectorAll(".archive-row[data-targets][data-search]"));
    const count = document.querySelector("[data-result-count]");
    const summary = document.querySelector("[data-filter-summary]");
    const notice = document.querySelector("[data-filter-notice]");
    const empty = document.querySelector("[data-empty-state]");
    if (!form || !count || !summary || !notice || !empty || rows.length === 0) {
      return false;
    }

    const search = form.elements.q;
    const checkboxes = Array.from(form.querySelectorAll('input[name="target"]'));
    const knownTargets = new Map(checkboxes.map(function (checkbox) {
      return [checkbox.value, checkbox.nextElementSibling.textContent];
    }));
    const editions = rows.map(function (row) {
      return {
        row: row,
        targets: row.dataset.targets.split(/\s+/).filter(Boolean),
        search: row.dataset.search
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

    function render(state) {
      const selectedTargets = normalizeTargets(state.targets || []);
      const selectedSet = new Set(selectedTargets);
      const query = String(state.query || "").trim();
      search.value = query;
      checkboxes.forEach(function (checkbox) {
        checkbox.checked = selectedSet.has(checkbox.value);
      });

      const visible = new Set(filterEditions(editions, {
        targets: selectedTargets,
        query: query
      }));
      editions.forEach(function (edition) {
        edition.row.hidden = !visible.has(edition);
      });
      empty.hidden = visible.size !== 0;

      count.textContent = visible.size + " of " + editions.length + " editions";
      const knownLabels = selectedTargets.filter(function (target) {
        return knownTargets.has(target);
      }).map(function (target) {
        return knownTargets.get(target);
      });
      const unknownTargets = selectedTargets.filter(function (target) {
        return !knownTargets.has(target);
      });
      const parts = [];
      parts.push(selectedTargets.length === 0 ? "All targets" : "Targets: " + knownLabels.concat(unknownTargets).join(" or "));
      if (query) {
        parts.push("Search: \u201c" + query + "\u201d");
      }
      summary.textContent = parts.join(". ");

      notice.hidden = unknownTargets.length === 0;
      notice.textContent = unknownTargets.length === 0 ? "" : "Unknown target " + (unknownTargets.length === 1 ? "filter" : "filters") + ": " + unknownTargets.join(", ") + ". " + (knownLabels.length === 0 ? "No edition can match until you clear or replace it." : "It remains part of the applied target filter.");
    }

    function push(state) {
      const url = serializeURL(browser.location.href, state);
      browser.history.pushState(null, "", url.pathname + url.search + url.hash);
    }

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      const state = readForm();
      render(state);
      push(state);
    });

    const clearButtons = Array.from(document.querySelectorAll("[data-clear-filters]"));
    const persistentClear = form.querySelector("[data-clear-filters]");
    clearButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        const state = { targets: [], query: "" };
        render(state);
        push(state);
        if (button !== persistentClear) {
          persistentClear.focus();
        }
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
    filterEditions: filterEditions,
    matchesEdition: matchesEdition,
    normalizeTargets: normalizeTargets,
    parseURL: parseURL,
    queryTerms: queryTerms,
    serializeURL: serializeURL,
    init: init
  };
});
