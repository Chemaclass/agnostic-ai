(function (root, factory) {
  "use strict";

  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.CompareTargets = api;

  if (root.document) {
    api.init(root.document, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const SLOTS = ["a", "b", "c"];
  const DEFAULT_IDS = ["claude", "codex"];

  function writesByDefault(status) {
    return status === "native" || status === "mapped";
  }

  function writesNothing(status) {
    return status === "no" || status === "source";
  }

  function normalizeId(value) {
    return String(value || "").trim().toLowerCase();
  }

  function firstUnused(known, used, preferred) {
    return preferred.concat(known).find(function (id) {
      return known.includes(id) && !used.includes(id);
    });
  }

  // Resolves the URL to two or three distinct known targets. A missing or
  // unknown A or B falls back to the default pair, and an unusable C is dropped.
  function parseSelection(value, known) {
    const url = value instanceof URL ? value : new URL(value, "https://example.invalid/");
    const unknown = [];
    const ids = [];
    SLOTS.forEach(function (slot) {
      const raw = url.searchParams.get(slot);
      const id = normalizeId(raw);
      if (id && !known.includes(id)) {
        unknown.push(raw.trim());
      }
      if (slot === "c") {
        if (known.includes(id) && !ids.includes(id)) {
          ids.push(id);
        }
        return;
      }
      if (known.includes(id) && !ids.includes(id)) {
        ids.push(id);
        return;
      }
      const fallback = firstUnused(known, ids, DEFAULT_IDS);
      if (fallback) {
        ids.push(fallback);
      }
    });
    return { ids: ids, unknown: unknown };
  }

  function serializeSelection(value, ids) {
    const url = value instanceof URL ? new URL(value.href) : new URL(value, "https://example.invalid/");
    SLOTS.forEach(function (slot, index) {
      if (ids[index]) {
        url.searchParams.set(slot, ids[index]);
      } else {
        url.searchParams.delete(slot);
      }
    });
    return url;
  }

  // Puts id in the slot. When another slot already shows id, that slot takes
  // the slot's previous target, so the columns stay distinct. An empty id
  // clears the optional third slot. An empty third slot has no target to
  // hand over, so choosing a target another column shows leaves it empty.
  function chooseTarget(ids, slot, id) {
    const next = ids.slice();
    if (!id) {
      return slot === SLOTS.length - 1 ? next.slice(0, slot) : next;
    }
    const previous = next[slot];
    const clash = next.indexOf(id);
    if (clash !== -1 && clash !== slot && !previous) {
      return next;
    }
    next[slot] = id;
    if (clash !== -1 && clash !== slot) {
      next[clash] = previous;
    }
    return next;
  }

  // The targets another column already shows, which this slot must not pick.
  function takenElsewhere(ids, slot) {
    return ids.filter(function (id, index) {
      return id && index !== slot;
    });
  }

  function selectedTargets(data, ids) {
    return ids.map(function (id) {
      return data.targets.find(function (target) {
        return target.id === id;
      });
    }).filter(Boolean);
  }

  function compareRows(data, ids) {
    const targets = selectedTargets(data, ids);
    return data.features.map(function (feature, index) {
      const cells = targets.map(function (target) {
        return {
          target: target,
          status: target.statuses[index],
          paths: (target.paths && target.paths[index]) || []
        };
      });
      const differs = cells.some(function (cell) {
        return cell.status !== cells[0].status;
      });
      return { feature: feature, cells: cells, differs: differs };
    });
  }

  function summarize(data, ids) {
    const targets = selectedTargets(data, ids);
    const statusesOf = function (index) {
      return targets.map(function (target) {
        return target.statuses[index];
      });
    };
    const perTarget = targets.map(function (target, position) {
      const entry = { target: target, only: [], optIn: [], missing: [] };
      data.features.forEach(function (feature, index) {
        const statuses = statusesOf(index);
        const mine = statuses[position];
        const others = statuses.filter(function (_, other) {
          return other !== position;
        });
        if (writesByDefault(mine) && !others.some(writesByDefault)) {
          entry.only.push(feature);
        }
        if (mine === "opt-in") {
          entry.optIn.push(feature);
        }
        if (writesNothing(mine) && others.some(function (status) { return !writesNothing(status); })) {
          entry.missing.push(feature);
        }
      });
      return entry;
    });
    const none = data.features.filter(function (_, index) {
      return statusesOf(index).every(writesNothing);
    });
    return { targets: perTarget, none: none };
  }

  function listNames(names) {
    if (names.length <= 2) {
      return names.join(" and ");
    }
    return names.slice(0, -1).join(", ") + ", and " + names[names.length - 1];
  }

  function resultText(rows) {
    const names = rows.length > 0 ? rows[0].cells.map(function (cell) { return cell.target.name; }) : [];
    const differing = rows.filter(function (row) { return row.differs; }).length;
    return listNames(names) + ": " + differing + " of " + rows.length + " spec kinds differ.";
  }

  function init(document, browser) {
    const browserRoot = document.querySelector("[data-compare-browser]");
    if (!browserRoot) {
      return false;
    }
    const payload = browserRoot.querySelector("[data-compare-data]");
    const form = browserRoot.querySelector("[data-compare-form]");
    const head = browserRoot.querySelector("[data-compare-head]");
    const body = browserRoot.querySelector("[data-compare-body]");
    const summary = browserRoot.querySelector("[data-compare-summary]");
    const noneLine = browserRoot.querySelector("[data-compare-none]");
    const result = browserRoot.querySelector("[data-compare-result]");
    const notice = browserRoot.querySelector("[data-compare-notice]");
    const empty = browserRoot.querySelector("[data-compare-empty]");
    const fallback = browserRoot.querySelector("[data-compare-fallback]");
    const diffOnly = browserRoot.querySelector("[data-compare-diff-only]");
    if (!payload || !form || !head || !body || !summary || !noneLine || !result || !notice || !empty || !diffOnly) {
      return false;
    }

    let data;
    try {
      data = JSON.parse(payload.textContent);
    } catch (error) {
      return false;
    }
    const known = data.targets.map(function (target) { return target.id; });
    const labels = new Map(data.states.map(function (state) { return [state.id, state.label]; }));
    const selects = SLOTS.map(function (slot) { return form.elements[slot]; });
    let ids = [];

    function element(tag, className, text) {
      const node = document.createElement(tag);
      if (className) {
        node.className = className;
      }
      if (text !== undefined) {
        node.textContent = text;
      }
      return node;
    }

    function link(href, text) {
      const anchor = element("a", "", text);
      anchor.href = href;
      return anchor;
    }

    function stateBadge(cell) {
      const label = labels.get(cell.status) || cell.status;
      if (cell.status === "opt-in") {
        const anchor = link(cell.target.href + "#config-keys", label);
        anchor.className = "capability-state capability-state-opt-in capability-state-link";
        anchor.setAttribute("aria-label", label + " for " + cell.target.name + ". View the required config key.");
        return anchor;
      }
      return element("span", "capability-state capability-state-" + cell.status, label);
    }

    function renderHead(targets) {
      const first = element("th", "", "Spec kind");
      first.scope = "col";
      const cells = [first];
      targets.forEach(function (target) {
        const th = element("th");
        th.scope = "col";
        const anchor = link(target.href, "");
        anchor.appendChild(element("span", "", target.name));
        anchor.appendChild(element("code", "", target.id));
        th.appendChild(anchor);
        cells.push(th);
      });
      head.replaceChildren.apply(head, cells);
    }

    function renderBody(rows) {
      body.replaceChildren.apply(body, rows.map(function (row) {
        const tr = element("tr", row.differs ? "compare-row-differs" : "");
        tr.dataset.compareKind = row.feature.id;
        if (row.differs) {
          tr.dataset.differs = "";
        }
        const th = element("th");
        th.scope = "row";
        th.appendChild(link(row.feature.href, row.feature.label));
        if (row.feature.toolSpecific) {
          th.appendChild(element("span", "compare-kind-note", "Tool-specific"));
        }
        if (row.differs) {
          th.appendChild(element("span", "compare-differs", "Differs"));
        }
        tr.appendChild(th);
        row.cells.forEach(function (cell) {
          const td = element("td", "compare-cell");
          td.dataset.label = cell.target.name;
          td.appendChild(stateBadge(cell));
          if (cell.paths.length > 0) {
            const list = element("ul", "compare-paths");
            cell.paths.forEach(function (path) {
              const item = element("li");
              const code = element("code");
              path.split("/").forEach(function (part, index) {
                if (index > 0) {
                  code.appendChild(document.createTextNode("/"));
                  code.appendChild(document.createElement("wbr"));
                }
                code.appendChild(document.createTextNode(part));
              });
              item.appendChild(code);
              list.appendChild(item);
            });
            td.appendChild(list);
          }
          tr.appendChild(td);
        });
        return tr;
      }));
    }

    function appendLinks(parent, features, fallbackText) {
      if (features.length === 0) {
        parent.appendChild(document.createTextNode(fallbackText + "."));
        return;
      }
      features.forEach(function (feature, index) {
        if (index > 0) {
          parent.appendChild(document.createTextNode(", "));
        }
        parent.appendChild(link(feature.href, feature.label));
      });
      parent.appendChild(document.createTextNode("."));
    }

    function renderSummary(report) {
      summary.replaceChildren.apply(summary, report.targets.map(function (entry) {
        const name = entry.target.name;
        const block = element("div", "compare-summary-target");
        block.appendChild(element("h4", "", name));
        const list = element("ul");
        const only = element("li", "", "Only " + name + " writes by default: ");
        appendLinks(only, entry.only, "nothing");
        list.appendChild(only);
        if (entry.optIn.length > 0) {
          const optIn = element("li", "", name + " needs a ");
          optIn.appendChild(link(entry.target.href + "#config-keys", "config key"));
          optIn.appendChild(document.createTextNode(" for: "));
          appendLinks(optIn, entry.optIn, "nothing");
          list.appendChild(optIn);
        }
        const missing = element("li", "", name + " gets no output for: ");
        appendLinks(missing, entry.missing, "nothing another target gets");
        list.appendChild(missing);
        block.appendChild(list);
        const caveats = element("p");
        caveats.appendChild(link(entry.target.href, name + " paths, config keys, and caveats"));
        block.appendChild(caveats);
        return block;
      }));
      noneLine.replaceChildren();
      if (report.none.length > 0) {
        noneLine.appendChild(document.createTextNode("No compared target gets: "));
        appendLinks(noneLine, report.none, "");
      }
    }

    function renderControls() {
      selects.forEach(function (select, index) {
        const taken = takenElsewhere(ids, index);
        Array.from(select.options).forEach(function (option) {
          option.disabled = option.value !== "" && taken.includes(option.value);
        });
        select.value = ids[index] || "";
      });
    }

    function applyDiffFilter(rows) {
      Array.from(body.rows).forEach(function (tr, index) {
        tr.hidden = diffOnly.checked && !rows[index].differs;
      });
      empty.hidden = !(diffOnly.checked && rows.every(function (row) { return !row.differs; }));
    }

    function render(unknown) {
      const rows = compareRows(data, ids);
      renderControls();
      renderHead(selectedTargets(data, ids));
      renderBody(rows);
      applyDiffFilter(rows);
      renderSummary(summarize(data, ids));
      result.textContent = resultText(rows);
      notice.hidden = unknown.length === 0;
      notice.textContent = unknown.length === 0 ? "" : "Unknown target " + listNames(unknown) + ". Showing " + listNames(selectedTargets(data, ids).map(function (target) { return target.name; })) + " instead.";
    }

    function update(next) {
      ids = next;
      render([]);
      const url = serializeSelection(browser.location.href, ids);
      browser.history.replaceState(null, "", url.pathname + url.search + url.hash);
    }

    selects.forEach(function (select, index) {
      select.addEventListener("change", function () {
        update(chooseTarget(ids, index, select.value));
      });
    });

    diffOnly.addEventListener("change", function () {
      applyDiffFilter(compareRows(data, ids));
    });

    form.addEventListener("submit", function (event) {
      event.preventDefault();
    });

    browser.addEventListener("popstate", function () {
      const selection = parseSelection(browser.location.href, known);
      ids = selection.ids;
      render(selection.unknown);
    });

    const selection = parseSelection(browser.location.href, known);
    ids = selection.ids;
    render(selection.unknown);
    form.hidden = false;
    if (fallback) {
      fallback.hidden = true;
    }
    return true;
  }

  return {
    chooseTarget: chooseTarget,
    compareRows: compareRows,
    init: init,
    parseSelection: parseSelection,
    resultText: resultText,
    serializeSelection: serializeSelection,
    summarize: summarize,
    takenElsewhere: takenElsewhere
  };
});
