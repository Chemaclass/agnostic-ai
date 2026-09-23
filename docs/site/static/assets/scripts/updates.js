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

  // paginate clamps page into range, so a stale ?page= from a longer
  // result set lands on the last page instead of an empty one.
  function paginate(total, page, size) {
    const pages = Math.max(1, Math.ceil(total / size));
    const current = Math.min(Math.max(1, Math.floor(Number(page)) || 1), pages);
    const start = (current - 1) * size;
    return { page: current, pages: pages, start: start, end: Math.min(start + size, total) };
  }

  function parseURL(value) {
    const url = value instanceof URL ? value : new URL(value, "https://example.invalid/");
    const page = parseInt(url.searchParams.get("page") || "", 10);
    return {
      targets: normalizeTargets(url.searchParams.getAll("target")),
      query: (url.searchParams.get("q") || "").trim(),
      page: page > 0 ? page : 1
    };
  }

  function serializeURL(value, state) {
    const url = value instanceof URL ? new URL(value.href) : new URL(value, "https://example.invalid/");
    url.searchParams.delete("target");
    url.searchParams.delete("q");
    url.searchParams.delete("page");
    normalizeTargets(state.targets || []).forEach(function (target) {
      url.searchParams.append("target", target);
    });
    const query = String(state.query || "").trim().replace(/\s+/g, " ");
    if (query) {
      url.searchParams.set("q", query);
    }
    if (state.page > 1) {
      url.searchParams.set("page", String(state.page));
    }
    url.hash = "archive-title";
    return url;
  }

  function dismissesDropdown(details, node) {
    if (!details || !details.open) {
      return false;
    }
    return !node || !details.contains(node);
  }

  function init(document, browser) {
    const form = document.querySelector("[data-updates-filter]");
    const rows = Array.from(document.querySelectorAll(".archive-row[data-targets][data-search]"));
    const count = document.querySelector("[data-result-count]");
    const summary = document.querySelector("[data-filter-summary]");
    const notice = document.querySelector("[data-filter-notice]");
    const empty = document.querySelector("[data-empty-state]");
    const pager = document.querySelector("[data-pager]");
    if (!form || !count || !summary || !notice || !empty || rows.length === 0) {
      return false;
    }

    const pageSize = Math.max(1, parseInt(pager && pager.dataset.pageSize, 10) || rows.length);
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

    function pageLink(state, page, label, current) {
      const link = document.createElement("a");
      link.href = serializeURL(browser.location.href, Object.assign({}, state, { page: page })).href;
      link.textContent = label;
      link.dataset.page = String(page);
      if (current) {
        link.setAttribute("aria-current", "page");
      }
      return link;
    }

    function renderPager(state, slice) {
      if (!pager) {
        return;
      }
      pager.hidden = slice.pages <= 1;
      pager.replaceChildren();
      if (pager.hidden) {
        return;
      }
      const list = document.createElement("ol");
      if (slice.page > 1) {
        const item = document.createElement("li");
        item.className = "pager-step";
        item.append(pageLink(state, slice.page - 1, "Previous", false));
        list.append(item);
      }
      for (let page = 1; page <= slice.pages; page += 1) {
        const item = document.createElement("li");
        const link = pageLink(state, page, String(page), page === slice.page);
        link.setAttribute("aria-label", "Page " + page + " of " + slice.pages);
        item.append(link);
        list.append(item);
      }
      if (slice.page < slice.pages) {
        const item = document.createElement("li");
        item.className = "pager-step";
        item.append(pageLink(state, slice.page + 1, "Next", false));
        list.append(item);
      }
      pager.append(list);
    }

    let current = { targets: [], query: "", page: 1 };

    function render(state) {
      const selectedTargets = normalizeTargets(state.targets || []);
      const selectedSet = new Set(selectedTargets);
      const query = String(state.query || "").trim();
      search.value = query;
      checkboxes.forEach(function (checkbox) {
        checkbox.checked = selectedSet.has(checkbox.value);
      });

      const matches = filterEditions(editions, {
        targets: selectedTargets,
        query: query
      });
      const slice = paginate(matches.length, state.page, pageSize);
      const visible = new Set(matches.slice(slice.start, slice.end));
      editions.forEach(function (edition) {
        edition.row.hidden = !visible.has(edition);
      });
      empty.hidden = matches.length !== 0;
      current = { targets: selectedTargets, query: query, page: slice.page };
      renderPager(current, slice);

      count.textContent = matches.length + " of " + editions.length + " editions" +
        (slice.pages > 1 ? ", page " + slice.page + " of " + slice.pages : "");
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

    if (pager) {
      pager.addEventListener("click", function (event) {
        const link = event.target.closest("a[data-page]");
        if (!link || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0) {
          return;
        }
        event.preventDefault();
        const state = Object.assign({}, current, { page: Number(link.dataset.page) });
        render(state);
        push(state);
        const heading = document.getElementById("archive-title");
        // Move focus with the view so keyboard and screen reader users
        // start at the top of the new page, not on a link that is gone.
        if (heading) {
          heading.setAttribute("tabindex", "-1");
          heading.focus();
        }
      });
    }

    const clearButtons = Array.from(document.querySelectorAll("[data-clear-filters]"));
    const persistentClear = form.querySelector("[data-clear-filters]");
    clearButtons.forEach(function (button) {
      button.addEventListener("click", function () {
        const state = { targets: [], query: "", page: 1 };
        render(state);
        push(state);
        if (button !== persistentClear) {
          persistentClear.focus();
        }
      });
    });

    const targetFilter = form.querySelector("details.target-filter");
    if (targetFilter) {
      const targetSummary = targetFilter.querySelector("summary");
      const closeOnOutside = function (event) {
        if (dismissesDropdown(targetFilter, event.target)) {
          targetFilter.open = false;
        }
      };
      document.addEventListener("click", closeOnOutside);
      document.addEventListener("focusin", closeOnOutside);
      targetFilter.addEventListener("keydown", function (event) {
        if (event.key !== "Escape" || !targetFilter.open) {
          return;
        }
        targetFilter.open = false;
        if (targetSummary) {
          targetSummary.focus();
        }
      });
    }

    browser.addEventListener("popstate", function () {
      render(parseURL(browser.location.href));
    });

    render(parseURL(browser.location.href));
    form.hidden = false;
    return true;
  }

  return {
    dismissesDropdown: dismissesDropdown,
    filterEditions: filterEditions,
    paginate: paginate,
    matchesEdition: matchesEdition,
    normalizeTargets: normalizeTargets,
    parseURL: parseURL,
    queryTerms: queryTerms,
    serializeURL: serializeURL,
    init: init
  };
});
