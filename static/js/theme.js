/**
 * Theme: light | dark | auto.
 * Switching animates as a circular wipe expanding from the clicked control
 * (View Transitions API when available; CSS radial overlay fallback).
 */
(function () {
  var KEY = "wikibuild-theme";
  var root = document.documentElement;
  var animating = false;

  function current() {
    try {
      return localStorage.getItem(KEY) || "auto";
    } catch (e) {
      return "auto";
    }
  }

  function resolvedIsDark(mode) {
    if (mode === "dark") return true;
    if (mode === "light") return false;
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  function syncButtons(mode) {
    var btns = document.querySelectorAll("[data-theme-set]");
    for (var i = 0; i < btns.length; i++) {
      var b = btns[i];
      var on = b.getAttribute("data-theme-set") === mode;
      b.classList.toggle("is-active", on);
      b.setAttribute("aria-pressed", on ? "true" : "false");
    }
    var cycle = document.getElementById("theme-toggle");
    if (cycle) {
      var label = mode === "light" ? "淺色" : mode === "dark" ? "深色" : "自動";
      cycle.setAttribute("data-theme-mode", mode);
      cycle.textContent = "主題：" + label;
    }
  }

  function applyTheme(mode) {
    if (mode !== "light" && mode !== "dark" && mode !== "auto") mode = "auto";
    if (mode === "auto") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", mode);
    try {
      localStorage.setItem(KEY, mode);
    } catch (e) {}
    syncButtons(mode);
    try {
      document.dispatchEvent(new CustomEvent("wikibuild:theme", { detail: { mode: mode } }));
    } catch (e) {}
  }

  function setCoordsFromEvent(e) {
    var x = window.innerWidth / 2;
    var y = 24;
    if (e && e.clientX != null) {
      x = e.clientX;
      y = e.clientY;
    } else if (e && e.currentTarget && e.currentTarget.getBoundingClientRect) {
      var r = e.currentTarget.getBoundingClientRect();
      x = r.left + r.width / 2;
      y = r.top + r.height / 2;
    }
    root.style.setProperty("--theme-x", x + "px");
    root.style.setProperty("--theme-y", y + "px");
    return { x: x, y: y };
  }

  function maxRadius(x, y) {
    var w = window.innerWidth;
    var h = window.innerHeight;
    var dist = Math.max(
      Math.hypot(x, y),
      Math.hypot(w - x, y),
      Math.hypot(x, h - y),
      Math.hypot(w - x, h - y)
    );
    return Math.ceil(dist) + 8;
  }

  /** Circular wipe from click point using View Transitions when possible. */
  function transitionTo(mode, e) {
    if (animating || mode === current()) {
      applyTheme(mode);
      return;
    }
    var coords = setCoordsFromEvent(e);
    var r = maxRadius(coords.x, coords.y);
    root.style.setProperty("--theme-r", r + "px");

    var reduce =
      window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduce) {
      applyTheme(mode);
      return;
    }

    // Prefer View Transitions circular reveal (Chrome/Edge/Safari recent).
    if (typeof document.startViewTransition === "function") {
      animating = true;
      root.classList.add("theme-animating");
      var vt = document.startViewTransition(function () {
        applyTheme(mode);
      });
      vt.finished.finally(function () {
        root.classList.remove("theme-animating");
        animating = false;
      });
      return;
    }

    // Fallback: radial overlay expanding from the button, then swap theme.
    animating = true;
    var overlay = document.createElement("div");
    overlay.className = "theme-circle-overlay";
    // Paint the *target* theme colors on the expanding circle.
    var goingDark = resolvedIsDark(mode);
    // Match site.css tokens so the wipe colour equals the destination theme.
    overlay.style.background = goingDark ? "#0b0f14" : "#f4f5f7";
    overlay.style.left = coords.x + "px";
    overlay.style.top = coords.y + "px";
    document.body.appendChild(overlay);
    // Force layout then expand
    void overlay.offsetWidth;
    overlay.style.transform = "translate(-50%, -50%) scale(1)";
    overlay.style.width = r * 2 + "px";
    overlay.style.height = r * 2 + "px";

    var done = false;
    function finish() {
      if (done) return;
      done = true;
      applyTheme(mode);
      overlay.classList.add("is-done");
      setTimeout(function () {
        if (overlay.parentNode) overlay.parentNode.removeChild(overlay);
        animating = false;
      }, 200);
    }
    overlay.addEventListener("transitionend", finish);
    setTimeout(finish, 700);
  }

  // Initial paint (no animation).
  applyTheme(current());

  document.addEventListener("click", function (e) {
    var t = e.target;
    if (!t || !t.closest) return;
    var setBtn = t.closest("[data-theme-set]");
    if (setBtn) {
      e.preventDefault();
      transitionTo(setBtn.getAttribute("data-theme-set"), e);
      return;
    }
    var cycle = t.closest("#theme-toggle");
    if (cycle) {
      e.preventDefault();
      var cur = current();
      var next = cur === "light" ? "dark" : cur === "dark" ? "auto" : "light";
      transitionTo(next, e);
    }
  });
})();
