/**
 * Theme toggle: light | dark | auto (follows prefers-color-scheme).
 * Persists choice in localStorage under "wikibuild-theme".
 */
(function () {
  var KEY = "wikibuild-theme";
  var root = document.documentElement;
  var btn = document.getElementById("theme-toggle");

  function cycle(cur) {
    if (cur === "light") return "dark";
    if (cur === "dark") return "auto";
    return "light";
  }

  function label(mode) {
    if (mode === "light") return "淺色";
    if (mode === "dark") return "深色";
    return "自動";
  }

  function apply(mode) {
    if (mode === "auto" || !mode) {
      root.removeAttribute("data-theme");
      try {
        localStorage.setItem(KEY, "auto");
      } catch (e) {}
      if (btn) {
        btn.setAttribute("data-theme-mode", "auto");
        btn.textContent = "主題：" + label("auto");
        btn.setAttribute("aria-label", "切換主題（目前：自動）");
      }
      return;
    }
    root.setAttribute("data-theme", mode);
    try {
      localStorage.setItem(KEY, mode);
    } catch (e) {}
    if (btn) {
      btn.setAttribute("data-theme-mode", mode);
      btn.textContent = "主題：" + label(mode);
      btn.setAttribute("aria-label", "切換主題（目前：" + label(mode) + "）");
    }
  }

  function current() {
    try {
      return localStorage.getItem(KEY) || "auto";
    } catch (e) {
      return "auto";
    }
  }

  // Sync button label after FOUC-prevention head script already applied theme.
  apply(current());

  if (btn) {
    btn.addEventListener("click", function () {
      apply(cycle(current()));
    });
  }
})();
