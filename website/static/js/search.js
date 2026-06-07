/*
 * ⌘K / Ctrl+K documentation search.
 *
 * Opens the native <dialog> rendered by partials/search.html, fetches
 * the home page's JSON output (the search index) once on first open,
 * and matches it in the browser. No external service, no build-time
 * index tool — the index is plain JSON Hugo emits at /index.json.
 *
 * Pairs with layouts/partials/search.html and the ".search-" block in
 * static/css/app.css. End-to-end behavior is covered by
 * e2e/tests/search.spec.ts.
 */
(() => {
  "use strict";

  const MAX_RESULTS = 8;

  function init() {
    const trigger = document.querySelector("[data-search-open]");
    const dialog = document.querySelector("[data-search-dialog]");
    // Native <dialog> is required for the modal backdrop, focus trap,
    // and Esc-to-close. Without it, leave the (hidden) trigger inert.
    if (!trigger || !dialog || typeof dialog.showModal !== "function") return;

    const input = dialog.querySelector("[data-search-input]");
    const list = dialog.querySelector("[data-search-results]");
    const statusEl = dialog.querySelector("[data-search-status]");
    const indexURL = dialog.getAttribute("data-search-index");

    let docs = null; // loaded index entries
    let loading = null; // in-flight fetch promise
    let results = []; // current result entries
    let active = -1; // active result index, -1 = none

    // ── platform-aware key hint ──────────────────────────────────────
    // Mac shows ⌘K; everything else shows Ctrl K. The markup ships the
    // ⌘ glyph; rewrite it off-Mac so the hint matches the real binding.
    const isMac = /mac|iphone|ipad|ipod/i.test(
      navigator.platform || navigator.userAgent || ""
    );
    if (!isMac) {
      const cmd = trigger.querySelector("[data-search-cmd]");
      if (cmd) cmd.textContent = "Ctrl";
    }

    // ── index loading ────────────────────────────────────────────────
    function loadIndex() {
      if (docs) return Promise.resolve(docs);
      if (loading) return loading;
      loading = fetch(indexURL)
        .then((r) => {
          if (!r.ok) throw new Error("HTTP " + r.status);
          return r.json();
        })
        .then((data) => {
          docs = Array.isArray(data) ? data : [];
          return docs;
        })
        .catch((err) => {
          // Let a later open retry the fetch.
          loading = null;
          throw err;
        });
      return loading;
    }

    // ── matching ─────────────────────────────────────────────────────
    function scoreDoc(doc, terms) {
      const title = (doc.title || "").toLowerCase();
      const summary = (doc.summary || "").toLowerCase();
      const body = (doc.body || "").toLowerCase();
      let score = 0;
      for (const t of terms) {
        let s = 0;
        const inTitle = title.indexOf(t);
        if (inTitle !== -1) {
          s += 12;
          if (inTitle === 0) s += 10; // prefix
          else if (/\s/.test(title.charAt(inTitle - 1))) s += 5; // word start
        }
        if (summary.indexOf(t) !== -1) s += 4;
        if (body.indexOf(t) !== -1) s += 1;
        // AND semantics: every term must appear somewhere.
        if (s === 0) return 0;
        score += s;
      }
      return score;
    }

    function search(query) {
      const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
      if (!docs || terms.length === 0) return [];
      const scored = [];
      for (const doc of docs) {
        const score = scoreDoc(doc, terms);
        if (score > 0) scored.push({ doc, score });
      }
      scored.sort((a, b) => b.score - a.score);
      return scored.slice(0, MAX_RESULTS).map((s) => s.doc);
    }

    // ── rendering ────────────────────────────────────────────────────
    // Wrap each matched term in <mark>, building DOM nodes (never
    // innerHTML) so the query string is escaped, not interpreted.
    function highlight(text, terms) {
      const frag = document.createDocumentFragment();
      if (!text) return frag;
      const lower = text.toLowerCase();
      let i = 0;
      while (i < text.length) {
        let next = -1;
        let len = 0;
        for (const t of terms) {
          const at = lower.indexOf(t, i);
          if (at !== -1 && (next === -1 || at < next)) {
            next = at;
            len = t.length;
          }
        }
        if (next === -1) {
          frag.appendChild(document.createTextNode(text.slice(i)));
          break;
        }
        if (next > i) {
          frag.appendChild(document.createTextNode(text.slice(i, next)));
        }
        const mark = document.createElement("mark");
        mark.textContent = text.slice(next, next + len);
        frag.appendChild(mark);
        i = next + len;
      }
      return frag;
    }

    function render(query) {
      const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
      list.textContent = "";
      active = -1;
      results = [];
      input.removeAttribute("aria-activedescendant");

      if (terms.length === 0) {
        setStatus("");
        input.setAttribute("aria-expanded", "false");
        return;
      }
      if (!docs) {
        // The index is still loading; open() re-renders this query
        // once the fetch resolves, so a query typed before the index
        // lands is not silently dropped.
        setStatus("Loading…");
        input.setAttribute("aria-expanded", "false");
        return;
      }

      results = search(query);
      input.setAttribute("aria-expanded", results.length ? "true" : "false");

      if (results.length === 0) {
        setStatus("No results for “" + query + "”.");
        return;
      }
      setStatus("");

      results.forEach((doc, idx) => {
        const li = document.createElement("li");
        li.className = "search-result";
        li.id = "search-result-" + idx;
        li.setAttribute("role", "option");
        li.setAttribute("aria-selected", "false");

        const a = document.createElement("a");
        a.className = "search-result-link";
        a.href = doc.href;

        const head = document.createElement("span");
        head.className = "search-result-head";
        const title = document.createElement("span");
        title.className = "search-result-title";
        title.appendChild(highlight(doc.title || "", terms));
        head.appendChild(title);
        if (doc.section) {
          const sec = document.createElement("span");
          sec.className = "search-result-section";
          sec.textContent = doc.section;
          head.appendChild(sec);
        }
        a.appendChild(head);

        if (doc.summary) {
          const sum = document.createElement("span");
          sum.className = "search-result-summary";
          sum.appendChild(highlight(doc.summary, terms));
          a.appendChild(sum);
        }

        // Pointer hover mirrors keyboard focus so the two never fight.
        li.addEventListener("mousemove", () => setActive(idx));
        li.appendChild(a);
        list.appendChild(li);
      });

      setActive(0);
    }

    function setStatus(msg) {
      statusEl.textContent = msg;
      statusEl.hidden = !msg;
    }

    function setActive(idx) {
      const items = list.querySelectorAll(".search-result");
      if (items.length === 0) {
        active = -1;
        return;
      }
      active = (idx + items.length) % items.length;
      items.forEach((el, i) => {
        const on = i === active;
        el.classList.toggle("is-active", on);
        el.setAttribute("aria-selected", on ? "true" : "false");
        if (on) {
          input.setAttribute("aria-activedescendant", el.id);
          el.scrollIntoView({ block: "nearest" });
        }
      });
    }

    function go() {
      const doc = results[active] || results[0];
      if (doc) window.location.assign(doc.href);
    }

    // ── open / close ─────────────────────────────────────────────────
    function open() {
      if (dialog.open) return;
      input.value = "";
      render("");
      dialog.showModal();
      document.documentElement.classList.add("search-active");
      input.focus();
      loadIndex()
        // Re-render the current query once the index lands — the user
        // may have typed while the fetch was still in flight.
        .then(() => render(input.value))
        .catch(() => setStatus("Search is unavailable right now."));
    }

    function close() {
      if (dialog.open) dialog.close();
    }

    dialog.addEventListener("close", () => {
      document.documentElement.classList.remove("search-active");
    });

    // Backdrop click: a modal <dialog> reports the click on the dialog
    // element itself (not its inner panel) when the backdrop is hit.
    dialog.addEventListener("click", (e) => {
      if (e.target === dialog) close();
    });

    trigger.addEventListener("click", open);

    const closeBtn = dialog.querySelector("[data-search-close]");
    if (closeBtn) closeBtn.addEventListener("click", close);

    input.addEventListener("input", () => render(input.value));

    input.addEventListener("keydown", (e) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          setActive(active + 1);
          break;
        case "ArrowUp":
          e.preventDefault();
          setActive(active - 1);
          break;
        case "Enter":
          if (results.length) {
            e.preventDefault();
            go();
          }
          break;
        // Esc is handled natively by <dialog>.
        default:
          break;
      }
    });

    // ── global shortcuts ─────────────────────────────────────────────
    document.addEventListener("keydown", (e) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        dialog.open ? close() : open();
        return;
      }
      // "/" focuses search, but only when the user is not already
      // typing into a field and the dialog is closed.
      if (e.key === "/" && !dialog.open && !isTypingTarget(e.target)) {
        e.preventDefault();
        open();
      }
    });
  }

  function isTypingTarget(el) {
    if (!el) return false;
    const tag = el.tagName;
    return (
      tag === "INPUT" ||
      tag === "TEXTAREA" ||
      tag === "SELECT" ||
      el.isContentEditable
    );
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
