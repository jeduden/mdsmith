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
    // The dialog can exist without its inner controls if the partial is
    // edited; bail rather than throw on the first open().
    if (!input || !list || !statusEl) return;

    let docs = null; // loaded index entries (lowercased fields cached)
    let loading = null; // in-flight fetch promise
    let results = []; // current result entries
    let resultEls = []; // rendered <li> nodes, parallel to results
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
          // Lowercase once at load, not once per keystroke: scoreDoc
          // reads the cached _title/_summary and the body (which is
          // only ever matched, never displayed, so it is folded to
          // lowercase in place).
          for (const d of docs) {
            d._title = (d.title || "").toLowerCase();
            d._summary = (d.summary || "").toLowerCase();
            d.body = (d.body || "").toLowerCase();
          }
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
      let score = 0;
      for (const t of terms) {
        let s = 0;
        const inTitle = doc._title.indexOf(t);
        if (inTitle !== -1) {
          s += 12;
          if (inTitle === 0) s += 10; // prefix
          else if (/\s/.test(doc._title.charAt(inTitle - 1))) s += 5; // word start
        }
        if (doc._summary.indexOf(t) !== -1) s += 4;
        if (doc.body.indexOf(t) !== -1) s += 1;
        // AND semantics: every term must appear somewhere.
        if (s === 0) return 0;
        score += s;
      }
      return score;
    }

    function search(terms) {
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
    // Compile the case-insensitive matcher for a query once per render.
    // Terms are ordered longest-first so a shorter term cannot mask part
    // of a longer overlapping one. Returns null for an empty query.
    function termsRegExp(terms) {
      if (terms.length === 0) return null;
      const ordered = terms.slice().sort((a, b) => b.length - a.length);
      return new RegExp("(" + ordered.map(escapeRegExp).join("|") + ")", "gi");
    }

    // Wrap each matched term in <mark>, building DOM text nodes (never
    // innerHTML) so the query is escaped, not interpreted. `re` is the
    // shared matcher from termsRegExp, compiled once per render.
    function highlight(text, re) {
      const frag = document.createDocumentFragment();
      if (!text) return frag;
      if (!re) {
        frag.appendChild(document.createTextNode(text));
        return frag;
      }
      // String.split with a capturing group yields [text, match, text,
      // …]; odd indices are the captured matches. split() ignores the
      // regex's lastIndex, so one compiled re is safely reused per call.
      text.split(re).forEach((part, i) => {
        if (part === "") return;
        if (i % 2 === 1) {
          const mark = document.createElement("mark");
          mark.textContent = part;
          frag.appendChild(mark);
        } else {
          frag.appendChild(document.createTextNode(part));
        }
      });
      return frag;
    }

    function render(query) {
      const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
      list.textContent = "";
      results = [];
      resultEls = [];
      active = -1;
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

      results = search(terms);
      input.setAttribute("aria-expanded", results.length ? "true" : "false");

      if (results.length === 0) {
        setStatus("No results for “" + query + "”.");
        return;
      }
      setStatus("");

      const re = termsRegExp(terms);
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
        title.appendChild(highlight(doc.title || "", re));
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
          sum.appendChild(highlight(doc.summary, re));
          a.appendChild(sum);
        }

        // Pointer hover mirrors keyboard focus so the two never fight.
        li.addEventListener("mousemove", () => setActive(idx));
        li.appendChild(a);
        list.appendChild(li);
        resultEls.push(li);
      });

      setActive(0);
    }

    function setStatus(msg) {
      statusEl.textContent = msg;
      statusEl.hidden = !msg;
    }

    function setActive(idx) {
      if (resultEls.length === 0) {
        active = -1;
        return;
      }
      // Only the previously- and newly-active rows change.
      if (active >= 0 && resultEls[active]) {
        resultEls[active].classList.remove("is-active");
        resultEls[active].setAttribute("aria-selected", "false");
      }
      active = (idx + resultEls.length) % resultEls.length;
      const el = resultEls[active];
      el.classList.add("is-active");
      el.setAttribute("aria-selected", "true");
      input.setAttribute("aria-activedescendant", el.id);
      el.scrollIntoView({ block: "nearest" });
    }

    function go() {
      // active is always a valid index when results is non-empty (render
      // calls setActive(0)), and Enter only calls go() under that guard.
      const doc = results[active];
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
        // may have typed while the fetch was in flight. Guard on
        // dialog.open so a fetch that resolves after the dialog was
        // closed does not repopulate a hidden listbox.
        .then(() => {
          if (dialog.open) render(input.value);
        })
        .catch(() => {
          if (dialog.open) setStatus("Search is unavailable right now.");
        });
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
        case "Escape":
          // WebKit consumes Escape on a non-empty type=search input to
          // clear the field, so it never reaches the <dialog>. Close
          // explicitly for consistent Escape-to-close across browsers.
          e.preventDefault();
          close();
          break;
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

  function escapeRegExp(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
