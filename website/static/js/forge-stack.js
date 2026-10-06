// Replays the homepage terminal: `mdsmith check` -> `fix` -> `check`.
//
// The transcript is server-rendered in full by
// layouts/partials/forge-stack.html, so with JavaScript off (or
// with prefers-reduced-motion set) the visitor reads the finished
// run and the intro.md card shows the fixed file. This script only
// animates that markup: it reads each `.fx-line`, types the
// `.fx-cmd` of prompt lines one character per tick, reveals output
// lines after their `data-wait` ticks, and toggles `is-fixed` on the
// stack when the `data-fix` line lands so the intro.md card behind
// the terminal switches to its fixed text. The loop runs until the
// visitor presses the pause control (WCAG 2.2.2), and it skips ticks
// while the tab is hidden.
(function () {
  "use strict";

  var TICK_MS = 65;
  var HOLD_TICKS = 80;

  function reducedMotion() {
    return !!(window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  }

  function init(stack) {
    if (reducedMotion()) return;
    var lines = Array.prototype.slice.call(stack.querySelectorAll(".fx-line"));
    if (!lines.length) return;

    var steps = lines.map(function (el) {
      var typed = el.hasAttribute("data-type");
      var cmd = typed ? el.querySelector(".fx-cmd") : null;
      var text = cmd ? cmd.textContent : "";
      var gap = parseInt(el.getAttribute("data-gap") || "4", 10);
      var wait = parseInt(el.getAttribute("data-wait") || "2", 10);
      return {
        el: el,
        cmd: cmd,
        text: text,
        typed: typed && cmd !== null,
        gap: gap,
        fix: el.hasAttribute("data-fix"),
        ticks: typed ? gap + text.length + 4 : wait
      };
    });
    var total = HOLD_TICKS + steps.reduce(function (n, s) { return n + s.ticks; }, 0);

    var cursor = document.createElement("span");
    cursor.className = "fx-cursor";
    cursor.setAttribute("aria-hidden", "true");
    cursor.textContent = " ";

    var tick = 0;
    var timer = null;

    function render() {
      var t = tick % total;
      var at = 0;
      var fixed = false;
      if (cursor.parentNode) cursor.parentNode.removeChild(cursor);
      steps.forEach(function (s) {
        var start = at;
        var end = at + s.ticks;
        at = end;
        if (s.typed) {
          var shown = t >= start;
          s.el.hidden = !shown;
          if (shown) {
            var n = Math.max(0, Math.min(s.text.length, t - start - s.gap));
            s.cmd.textContent = s.text.slice(0, n);
            if (t < end) s.el.appendChild(cursor);
          }
        } else {
          var on = t >= end - 1;
          s.el.hidden = !on;
          if (on && s.fix) fixed = true;
        }
      });
      stack.classList.toggle("is-fixed", fixed);
    }

    function advance() {
      if (document.hidden) return;
      tick += 1;
      render();
    }

    var toggle = stack.querySelector("[data-forge-toggle]");

    function play() {
      if (timer !== null) return;
      timer = window.setInterval(advance, TICK_MS);
      stack.setAttribute("data-playing", "true");
      if (toggle) toggle.setAttribute("aria-pressed", "false");
    }

    function pause() {
      if (timer !== null) window.clearInterval(timer);
      timer = null;
      stack.setAttribute("data-playing", "false");
      if (toggle) toggle.setAttribute("aria-pressed", "true");
    }

    if (toggle) {
      toggle.hidden = false;
      toggle.addEventListener("click", function () {
        if (timer === null) play(); else pause();
      });
    }

    render();
    play();
  }

  document.addEventListener("DOMContentLoaded", function () {
    var stacks = document.querySelectorAll("[data-forge-stack]");
    Array.prototype.forEach.call(stacks, init);
  });
})();
