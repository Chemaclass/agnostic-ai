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
  const PER_PAGE_LIMIT = 3;
  const HEAD_EXACT = 30;
  const HEAD_PREFIX = 24;
  const TITLE_EXACT = 12;
  const TITLE_PREFIX = 5;
  const BODY_BASE = 4;
  const BODY_HIT_CAP = 4;
  const COVERAGE_WEIGHT = 40;
  const PHRASE_HEAD = 20;
  const PHRASE_BODY = 10;
  const LANDING_BONUS = 15;
  const SLUG_BONUS = 10;
  const GROUP_RANK = { Docs: 0, Targets: 1, Updates: 2 };
  const DEFAULT_GROUP_RANK = 1;
  const GROUP_WEIGHT = { Updates: 0.7 };
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

  // `zola serve` appends a live-reload script after the JSON, since the index is served as HTML.
  function parseIndex(text) {
    return JSON.parse(text.slice(0, text.lastIndexOf("]") + 1));
  }

  // Fold a plural onto its singular so "hooks" and "hook" reach the
  // same entries. Short words and an "ss" ending keep their final s.
  function stem(word) {
    return word.length > 3 && /[a-rt-z]s$/.test(word) ? word.slice(0, -1) : word;
  }

  function stemPhrase(text) {
    return text.split(" ").filter(Boolean).map(stem).join(" ");
  }

  function pageUrl(url) {
    return String(url || "").split("#")[0];
  }

  // The last path segment names the topic when the title does not:
  // /docs/migration/ is titled "Import existing tool configuration".
  function slugWords(url) {
    const segments = pageUrl(url).split("/").filter(Boolean);
    const last = segments.length === 0 ? "" : segments[segments.length - 1];
    return normalize(last.replace(/[-_]+/g, " "));
  }

  function wordCount(text) {
    return normalize(text).split(" ").filter(Boolean).length || 1;
  }

  function prepare(entries) {
    return (entries || []).map(function (entry) {
      const isPage = !entry.heading;
      const slug = stemPhrase(slugWords(entry.url));
      const head = isPage ? normalize(entry.title) + " " + slug : normalize(entry.heading);
      return {
        entry: entry,
        isPage: isPage,
        page: pageUrl(entry.url),
        slug: slug,
        head: " " + stemPhrase(head) + " ",
        headWords: wordCount(isPage ? entry.title : entry.heading),
        title: isPage ? " " : " " + stemPhrase(normalize(entry.title)) + " ",
        body: " " + stemPhrase(normalize(entry.body)) + " "
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

  // A query whose words sit together is about that phrase, not about
  // each word on its own.
  function phraseScore(prepared, terms) {
    if (terms.length < 2) {
      return 0;
    }
    const phrase = " " + terms.join(" ");
    if (prepared.head.indexOf(phrase) !== -1) {
      return PHRASE_HEAD;
    }
    return prepared.body.indexOf(phrase) !== -1 ? PHRASE_BODY : 0;
  }

  // Coverage is what keeps a short exact heading ahead of a long one
  // that merely contains the term: "Hooks" covers the whole heading,
  // "permissionMode and agent hooks support by target" covers a sixth
  // of it. Sections inherit only a little of their page title, or every
  // section of a matching page outranks the page itself.
  function scoreEntry(prepared, rawTerms) {
    if (!rawTerms || rawTerms.length === 0) {
      return 0;
    }
    const terms = rawTerms.map(stem);
    let total = 0;
    let covered = 0;
    for (const term of terms) {
      const head = fieldScore(prepared.head, term, HEAD_EXACT, HEAD_PREFIX);
      if (head > 0) {
        covered += 1;
      }
      const bodyHits = countWordStarts(prepared.body, " " + term, BODY_HIT_CAP);
      const score = head +
        fieldScore(prepared.title, term, TITLE_EXACT, TITLE_PREFIX) +
        (bodyHits > 0 ? BODY_BASE + bodyHits : 0);
      if (score === 0) {
        return 0;
      }
      total += score;
    }
    total += COVERAGE_WEIGHT * covered / prepared.headWords;
    total += phraseScore(prepared, terms);
    if (prepared.isPage && covered > 0) {
      total += LANDING_BONUS;
      if (prepared.slug === terms.join(" ")) {
        total += SLUG_BONUS;
      }
    }
    return total * (GROUP_WEIGHT[prepared.entry.group] || 1);
  }

  function groupRank(item) {
    const rank = GROUP_RANK[item.entry.group];
    return rank === undefined ? DEFAULT_GROUP_RANK : rank;
  }

  // Score first, then the group a reader most likely wants, then the
  // page ahead of one of its own sections, then index order. Index
  // order alone is what made equal scores look arbitrary.
  function compareResults(a, b) {
    if (b.score !== a.score) {
      return b.score - a.score;
    }
    const group = groupRank(a.item) - groupRank(b.item);
    if (group !== 0) {
      return group;
    }
    if (a.item.isPage !== b.item.isPage) {
      return a.item.isPage ? -1 : 1;
    }
    return a.index - b.index;
  }

  // One page filling every slot with its own sections hides the rest of
  // the corpus, so a page contributes at most PER_PAGE_LIMIT entries
  // before every other page has had its turn. Entries held back that way
  // still fill slots the rest of the corpus leaves empty, in rank order,
  // rather than shrinking the result list.
  function capPerPage(ranked, limit) {
    const counts = {};
    const entries = [];
    const overflow = [];
    for (const result of ranked) {
      if (entries.length >= limit) {
        return entries;
      }
      const seen = counts[result.item.page] || 0;
      if (seen >= PER_PAGE_LIMIT) {
        overflow.push(result.item.entry);
        continue;
      }
      counts[result.item.page] = seen + 1;
      entries.push(result.item.entry);
    }
    for (const entry of overflow) {
      if (entries.length >= limit) {
        break;
      }
      entries.push(entry);
    }
    return entries;
  }

  function search(prepared, query, limit) {
    if (normalize(query).length < 2) {
      return [];
    }
    const terms = tokenize(query);
    const ranked = (prepared || [])
      .map(function (item, index) {
        return { item: item, score: scoreEntry(item, terms), index: index };
      })
      .filter(function (result) {
        return result.score > 0;
      })
      .sort(compareResults);
    return capPerPage(ranked, limit === undefined ? RESULT_LIMIT : limit);
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
          return response.text();
        })
        .then(function (text) {
          prepared = prepare(parseIndex(text));
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
      // Stems, not raw terms: a result can match on "hook" while the
      // reader typed "hooks", and the snippet should still find it.
      render(found, tokenize(query).map(stem));
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
    parseIndex: parseIndex,
    prepare: prepare,
    scoreEntry: scoreEntry,
    search: search,
    shortcutHint: shortcutHint,
    stem: stem,
    teaser: teaser,
    tokenize: tokenize
  };
});
