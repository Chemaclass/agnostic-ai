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

  const FORMAT_NAMES = { md: "Markdown", mdc: "Markdown", toml: "TOML", json: "JSON", yaml: "YAML", yml: "YAML" };

  // The file format a path's extension names. A dotfile such as .aiignore has
  // no extension, so it names no format.
  function pathFormat(path) {
    const base = String(path).split("/").pop();
    const dot = base.lastIndexOf(".");
    if (dot <= 0 || dot === base.length - 1) {
      return "";
    }
    const extension = base.slice(dot + 1).toLowerCase();
    return FORMAT_NAMES[extension] || extension;
  }

  // The distinct formats of a cell's paths, in path order.
  function pathFormats(paths) {
    return paths.map(pathFormat).filter(function (format, index, formats) {
      return format && formats.indexOf(format) === index;
    });
  }

  function formatKey(formats) {
    return formats.slice().sort().join("+");
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
    const unknownIds = [];
    const ids = [];
    SLOTS.forEach(function (slot) {
      const raw = url.searchParams.get(slot);
      const id = normalizeId(raw);
      if (id && !known.includes(id) && !unknownIds.includes(id)) {
        unknownIds.push(id);
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
        const paths = (target.paths && target.paths[index]) || [];
        return {
          target: target,
          status: target.statuses[index],
          paths: paths,
          formats: pathFormats(paths)
        };
      });
      const differs = cells.some(function (cell) {
        return cell.status !== cells[0].status;
      });
      // A format difference is reported only where the states agree, so the
      // two signals never overlap. Cells without a format do not take part.
      const keys = cells.filter(function (cell) { return cell.formats.length > 0; }).map(function (cell) {
        return formatKey(cell.formats);
      });
      const formatDiffers = !differs && keys.some(function (key) { return key !== keys[0]; });
      return { feature: feature, cells: cells, differs: differs, formatDiffers: formatDiffers };
    });
  }

  // For each compared target, the kinds it covers by default that another
  // column lacks (ahead), and the kinds another column covers by default
  // that it lacks (behind), each naming the other columns involved.
  function summarize(data, ids) {
    const targets = selectedTargets(data, ids);
    const perTarget = targets.map(function (target, position) {
      const entry = { target: target, ahead: [], behind: [] };
      data.features.forEach(function (feature, index) {
        const mine = target.statuses[index];
        const others = targets.filter(function (_, other) {
          return other !== position;
        }).map(function (other) {
          return { target: other, status: other.statuses[index] };
        });
        if (writesByDefault(mine)) {
          const lacking = others.filter(function (other) { return !writesByDefault(other.status); });
          if (lacking.length > 0) {
            entry.ahead.push({ feature: feature, others: lacking });
          }
          return;
        }
        const leaders = others.filter(function (other) { return writesByDefault(other.status); });
        if (leaders.length > 0) {
          entry.behind.push({ feature: feature, status: mine, others: leaders });
        }
      });
      return entry;
    });
    const none = data.features.filter(function (_, index) {
      return targets.every(function (target) { return writesNothing(target.statuses[index]); });
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
    const formats = rows.filter(function (row) { return row.formatDiffers; }).length;
    return listNames(names) + ": " + differing + " of " + rows.length + " spec kinds differ in support, " + formats + " in file format.";
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
    const summarySection = browserRoot.querySelector("[data-compare-summary-section]");
    const noneLine = browserRoot.querySelector("[data-compare-none]");
    const result = browserRoot.querySelector("[data-compare-result]");
    const notice = browserRoot.querySelector("[data-compare-notice]");
    const empty = browserRoot.querySelector("[data-compare-empty]");
    const fallback = browserRoot.querySelector("[data-compare-fallback]");
    const diffOnly = browserRoot.querySelector("[data-compare-diff-only]");
    if (!payload || !form || !head || !body || !summary || !summarySection || !noneLine || !result || !notice || !empty || !diffOnly) {
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
        const tr = element("tr", row.differs ? "compare-row-differs" : row.formatDiffers ? "compare-row-format-differs" : "");
        tr.dataset.compareKind = row.feature.id;
        if (row.differs) {
          tr.dataset.differs = "";
        }
        if (row.formatDiffers) {
          tr.dataset.formatDiffers = "";
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
        if (row.formatDiffers) {
          th.appendChild(element("span", "compare-format-differs", "Format differs"));
        }
        tr.appendChild(th);
        row.cells.forEach(function (cell) {
          const td = element("td", "compare-cell");
          td.dataset.label = cell.target.name;
          const state = element("div", "compare-cell-state");
          state.appendChild(stateBadge(cell));
          cell.formats.forEach(function (format) {
            state.appendChild(element("span", "compare-format", format));
          });
          td.appendChild(state);
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

    function names(others) {
      return listNames(others.map(function (other) { return other.target.name; }));
    }

    // "Codex needs a config key; Junie gets no output."
    function appendLacking(parent, lacking) {
      const optIn = lacking.filter(function (other) { return other.status === "opt-in"; });
      const nothing = lacking.filter(function (other) { return other.status !== "opt-in"; });
      optIn.forEach(function (other, index) {
        parent.appendChild(document.createTextNode((index > 0 ? "; " : "") + other.target.name + " needs a "));
        parent.appendChild(link(other.target.href + "#config-keys", "config key"));
      });
      if (nothing.length > 0) {
        const verb = nothing.length > 1 ? " get no output" : " gets no output";
        parent.appendChild(document.createTextNode((optIn.length > 0 ? "; " : "") + names(nothing) + verb));
      }
      parent.appendChild(document.createTextNode("."));
    }

    function kindItem(feature) {
      const item = element("li");
      item.appendChild(link(feature.href, feature.label));
      item.appendChild(document.createTextNode(": "));
      return item;
    }

    function kindCount(count) {
      return count + (count === 1 ? " kind" : " kinds");
    }

    function summaryList(direction, title, items) {
      const fragment = document.createDocumentFragment();
      fragment.appendChild(element("p", "compare-summary-label", title + " " + kindCount(items.length)));
      const list = element("ul", "compare-summary-" + direction);
      items.forEach(function (item) { list.appendChild(item); });
      fragment.appendChild(list);
      return fragment;
    }

    function renderSummary(report) {
      summary.replaceChildren.apply(summary, report.targets.map(function (entry) {
        const name = entry.target.name;
        const block = element("div", "compare-summary-target");
        block.appendChild(element("h3", "", name));
        if (entry.ahead.length > 0) {
          block.appendChild(summaryList("ahead", "Ahead on", entry.ahead.map(function (gap) {
            const item = kindItem(gap.feature);
            appendLacking(item, gap.others);
            return item;
          })));
        }
        if (entry.behind.length > 0) {
          block.appendChild(summaryList("behind", "Behind on", entry.behind.map(function (gap) {
            const item = kindItem(gap.feature);
            if (gap.status === "opt-in") {
              item.appendChild(document.createTextNode("needs a "));
              item.appendChild(link(entry.target.href + "#config-keys", "config key"));
            } else {
              item.appendChild(document.createTextNode("no output"));
            }
            const verb = gap.others.length > 1 ? " write it by default." : " writes it by default.";
            item.appendChild(document.createTextNode("; " + names(gap.others) + verb));
            return item;
          })));
        }
        if (entry.ahead.length === 0 && entry.behind.length === 0) {
          block.appendChild(element("p", "", "Covers the same spec kinds by default as the other targets here."));
        }
        const caveats = element("p", "compare-summary-more");
        caveats.appendChild(link(entry.target.href, name + " paths, config keys, and caveats"));
        block.appendChild(caveats);
        return block;
      }));
      noneLine.replaceChildren();
      if (report.none.length > 0) {
        noneLine.appendChild(document.createTextNode("No compared target gets: "));
        report.none.forEach(function (feature, index) {
          if (index > 0) {
            noneLine.appendChild(document.createTextNode(", "));
          }
          noneLine.appendChild(link(feature.href, feature.label));
        });
        noneLine.appendChild(document.createTextNode("."));
      }
      summarySection.hidden = false;
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
      const shown = function (row) { return row.differs || row.formatDiffers; };
      Array.from(body.rows).forEach(function (tr, index) {
        tr.hidden = diffOnly.checked && !shown(rows[index]);
      });
      empty.hidden = !(diffOnly.checked && !rows.some(shown));
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
    pathFormat: pathFormat,
    pathFormats: pathFormats,
    resultText: resultText,
    serializeSelection: serializeSelection,
    summarize: summarize,
    takenElsewhere: takenElsewhere
  };
});
