(function (root, factory) {
  "use strict";

  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  root.SiteSearch = api;

  if (root.document) {
    api.init(root.document, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  "use strict";

  const RESULT_LIMIT = 10;
  const IDLE_STATUS = "Type to search the docs and updates.";
  const ENTITIES = { "&lt;": "<", "&gt;": ">", "&amp;": "&", "&quot;": "\"", "&#39;": "'" };

  function decodeEntities(text) {
    return String(text || "").replace(/&(?:lt|gt|amp|quot|#39);/g, function (entity) {
      return ENTITIES[entity];
    });
  }

  function normalize(text) {
    return decodeEntities(text).toLowerCase().replace(/[^\p{L}\p{N}._/-]+/gu, " ").trim();
  }

  function tokenize(query) {
    return normalize(query).split(" ").filter(Boolean);
  }

  function prepare(entries) {
    return (entries || []).map(function (entry) {
      return {
        entry: entry,
        title: " " + normalize(entry.title) + " ",
        heading: " " + normalize(entry.heading) + " ",
        body: " " + normalize(entry.body) + " "
      };
    });
  }

  function countWordStarts(haystack, needle, max) {
    let count = 0;
    let index = haystack.indexOf(needle);
    while (index !== -1 && count < max) {
      count += 1;
      index = haystack.indexOf(needle, index + needle.length);
    }
    return count;
  }

  function fieldScore(field, term, exactScore, prefixScore) {
    if (field.indexOf(" " + term + " ") !== -1) {
      return exactScore;
    }
    return field.indexOf(" " + term) !== -1 ? prefixScore : 0;
  }

  function scoreEntry(prepared, terms) {
    if (!terms || terms.length === 0) {
      return 0;
    }
    let total = 0;
    for (const term of terms) {
      const bodyHits = countWordStarts(prepared.body, " " + term, 5);
      const score = fieldScore(prepared.heading, term, 40, 20) +
        fieldScore(prepared.title, term, 30, 12) +
        (bodyHits > 0 ? 5 + bodyHits : 0);
      if (score === 0) {
        return 0;
      }
      total += score;
    }
    return prepared.entry.heading ? total : total + 3;
  }

  function search(prepared, query, limit) {
    if (normalize(query).length < 2) {
      return [];
    }
    const terms = tokenize(query);
    return (prepared || [])
      .map(function (item, index) {
        return { entry: item.entry, score: scoreEntry(item, terms), index: index };
      })
      .filter(function (result) {
        return result.score > 0;
      })
      .sort(function (a, b) {
        return b.score - a.score || a.index - b.index;
      })
      .slice(0, limit === undefined ? RESULT_LIMIT : limit)
      .map(function (result) {
        return result.entry;
      });
  }

  function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\/-]/g, "\\$&");
  }

  function teaser(body, terms, width) {
    const size = width || 160;
    const text = decodeEntities(body).replace(/\s+/g, " ").trim();
    if (!text) {
      return [];
    }

    const lower = text.toLowerCase();
    const words = (terms || []).filter(Boolean);
    let hit = -1;
    words.forEach(function (term) {
      const index = lower.indexOf(term);
      if (index !== -1 && (hit === -1 || index < hit)) {
        hit = index;
      }
    });

    let start = hit > 0 ? Math.max(0, hit - Math.floor(size / 3)) : 0;
    const end = Math.min(text.length, start + size);
    start = Math.max(0, Math.min(start, end - size));
    const excerpt = (start > 0 ? "…" : "") + text.slice(start, end) + (end < text.length ? "…" : "");

    if (words.length === 0) {
      return [{ text: excerpt, mark: false }];
    }
    const pattern = new RegExp(words.slice().sort(function (a, b) {
      return b.length - a.length;
    }).map(escapeRegExp).join("|"), "gi");

    const segments = [];
    let cursor = 0;
    let match;
    while ((match = pattern.exec(excerpt)) !== null) {
      if (match[0].length === 0) {
        pattern.lastIndex += 1;
        continue;
      }
      if (match.index > cursor) {
        segments.push({ text: excerpt.slice(cursor, match.index), mark: false });
      }
      segments.push({ text: match[0], mark: true });
      cursor = match.index + match[0].length;
    }
    if (cursor < excerpt.length) {
      segments.push({ text: excerpt.slice(cursor), mark: false });
    }
    return segments;
  }

  function shortcutHint(platform) {
    return /mac|iphone|ipad/i.test(String(platform || "")) ? "⌘K" : "Ctrl K";
  }

  function isTypingTarget(element) {
    if (!element || !element.tagName) {
      return false;
    }
    const tag = element.tagName.toLowerCase();
    return tag === "input" || tag === "textarea" || tag === "select" || element.isContentEditable === true;
  }

  function init(document, browser) {
    const dialog = document.querySelector("[data-search]");
    if (!dialog) {
      return false;
    }
    const form = dialog.querySelector("[data-search-form]");
    const input = dialog.querySelector("[data-search-input]");
    const status = dialog.querySelector("[data-search-status]");
    const results = dialog.querySelector("[data-search-results]");
    const closeButton = dialog.querySelector("[data-search-close]");
    if (!form || !input || !status || !results) {
      return false;
    }

    const navigator = browser.navigator || {};
    const userAgentData = navigator.userAgentData || {};
    Array.from(document.querySelectorAll("[data-search-hint]")).forEach(function (hint) {
      hint.textContent = shortcutHint(userAgentData.platform || navigator.platform);
    });

    let prepared = null;
    let loading = null;
    let opener = null;
    let matches = [];
    let selected = -1;
    let debounce = null;

    function isOpen() {
      return dialog.open;
    }

    function setStatus(text) {
      status.textContent = text;
    }

    function load() {
      if (loading) {
        return loading;
      }
      setStatus("Loading the index.");
      loading = browser.fetch(dialog.dataset.searchIndex)
        .then(function (response) {
          if (!response.ok) {
            throw new Error("search index request failed");
          }
          return response.json();
        })
        .then(function (entries) {
          prepared = prepare(entries);
          runQuery();
        })
        .catch(function () {
          loading = null;
          setStatus("Search is unavailable right now.");
        });
      return loading;
    }

    function select(index) {
      const options = results.querySelectorAll("[role=option]");
      selected = index;
      Array.from(options).forEach(function (option, position) {
        option.setAttribute("aria-selected", position === index ? "true" : "false");
      });
      const active = options[index];
      if (active) {
        input.setAttribute("aria-activedescendant", active.id);
        active.scrollIntoView({ block: "nearest" });
      } else {
        input.removeAttribute("aria-activedescendant");
      }
    }

    function render(entries, terms) {
      results.textContent = "";
      matches = entries;
      entries.forEach(function (entry, index) {
        const option = document.createElement("li");
        option.setAttribute("role", "option");
        option.id = "site-search-option-" + index;
        option.setAttribute("aria-selected", "false");

        const link = document.createElement("a");
        link.href = entry.url;

        const crumb = document.createElement("span");
        crumb.className = "site-search-crumb";
        crumb.textContent = entry.group + " / " + entry.title;

        const title = document.createElement("span");
        title.className = "site-search-title";
        title.textContent = entry.heading || entry.title;

        const snippet = document.createElement("span");
        snippet.className = "site-search-snippet";
        teaser(entry.body, terms).forEach(function (segment) {
          if (segment.mark) {
            const mark = document.createElement("mark");
            mark.textContent = segment.text;
            snippet.appendChild(mark);
          } else {
            snippet.appendChild(document.createTextNode(segment.text));
          }
        });

        link.appendChild(crumb);
        link.appendChild(title);
        link.appendChild(snippet);
        option.appendChild(link);
        results.appendChild(option);
      });
      input.setAttribute("aria-expanded", entries.length > 0 ? "true" : "false");
      select(-1);
    }

    function runQuery() {
      if (!prepared) {
        return;
      }
      const query = input.value;
      if (normalize(query).length < 2) {
        render([], []);
        setStatus(IDLE_STATUS);
        return;
      }
      const found = search(prepared, query, RESULT_LIMIT);
      render(found, tokenize(query));
      if (found.length === 0) {
        setStatus("Nothing found. Try another word.");
      } else {
        setStatus(found.length === 1 ? "1 result" : found.length + " results");
      }
    }

    function open(source) {
      if (isOpen()) {
        input.focus();
        input.select();
        return;
      }
      opener = source || document.activeElement;
      dialog.showModal();
      document.documentElement.classList.add("search-open");
      input.focus();
      input.select();
      if (prepared) {
        runQuery();
      } else {
        load();
      }
    }

    function handleClosed() {
      document.documentElement.classList.remove("search-open");
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
      if (opener && typeof opener.focus === "function" && opener !== document.body) {
        opener.focus();
      }
      opener = null;
    }

    function close() {
      if (!isOpen()) {
        return;
      }
      dialog.close();
    }

    function openSelected() {
      const entry = matches[selected >= 0 ? selected : 0];
      if (!entry) {
        return;
      }
      close();
      browser.location.assign(entry.url);
    }

    Array.from(document.querySelectorAll("[data-search-open]")).forEach(function (trigger) {
      trigger.addEventListener("pointerenter", load);
      trigger.addEventListener("focus", load);
      trigger.addEventListener("click", function () {
        open(trigger);
      });
    });

    document.addEventListener("keydown", function (event) {
      const key = event.key;
      if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && (key === "k" || key === "K")) {
        event.preventDefault();
        open(null);
        return;
      }
      if (key === "/" && !event.metaKey && !event.ctrlKey && !event.altKey && !isOpen() && !isTypingTarget(event.target)) {
        event.preventDefault();
        open(null);
      }
    });

    dialog.addEventListener("keydown", function (event) {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        if (matches.length === 0) {
          return;
        }
        event.preventDefault();
        const step = event.key === "ArrowDown" ? 1 : -1;
        const start = selected < 0 && step < 0 ? 0 : selected;
        select((start + step + matches.length) % matches.length);
      }
    });

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      openSelected();
    });

    input.addEventListener("input", function () {
      browser.clearTimeout(debounce);
      debounce = browser.setTimeout(runQuery, 100);
    });

    results.addEventListener("click", function (event) {
      if (event.target.closest("a")) {
        close();
      }
    });

    dialog.addEventListener("click", function (event) {
      if (event.target === dialog) {
        close();
      }
    });

    dialog.addEventListener("close", handleClosed);

    if (closeButton) {
      closeButton.addEventListener("click", close);
    }
    return true;
  }

  return {
    decodeEntities: decodeEntities,
    init: init,
    isTypingTarget: isTypingTarget,
    normalize: normalize,
    prepare: prepare,
    scoreEntry: scoreEntry,
    search: search,
    shortcutHint: shortcutHint,
    teaser: teaser,
    tokenize: tokenize
  };
});
