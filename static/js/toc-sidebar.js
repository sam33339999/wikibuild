/**
 * Floating TOC panel (does not reflow article width).
 * localStorage: wikibuild-toc-open = "0" | "1" (default: open ≥1100px, closed below)
 */
(function () {
  var KEY = "wikibuild-toc-open";
  var WIDE = 1100;

  var root = document.querySelector(".article-layout");
  var sidebar = document.getElementById("toc-sidebar");
  var backdrop = document.getElementById("toc-backdrop");
  var btnClose = document.getElementById("toc-close");
  var btnOpen = document.getElementById("toc-open"); // inline under title
  var btnFab = document.getElementById("toc-fab");
  if (!root || !sidebar) return;

  function lsGet() {
    try {
      return localStorage.getItem(KEY);
    } catch (e) {
      return null;
    }
  }
  function lsSet(v) {
    try {
      if (v == null) localStorage.removeItem(KEY);
      else localStorage.setItem(KEY, v);
    } catch (e) {}
  }

  function isWide() {
    return window.matchMedia("(min-width: " + WIDE + "px)").matches;
  }

  function defaultOpen() {
    var stored = lsGet();
    if (stored === "1") return true;
    if (stored === "0") return false;
    return isWide();
  }

  var open = defaultOpen();

  function setExpanded(btn, val) {
    if (btn) btn.setAttribute("aria-expanded", val ? "true" : "false");
  }

  function apply() {
    root.classList.toggle("toc-open", open);
    root.classList.toggle("toc-closed", !open);

    sidebar.hidden = !open;
    if (backdrop) {
      // Backdrop only when panel would cover content (narrow).
      backdrop.hidden = open ? isWide() : true;
    }

    // Inline "目錄" in article head: hide while open (panel already visible).
    if (btnOpen) {
      btnOpen.hidden = open;
      setExpanded(btnOpen, open);
    }
    // FAB: only when closed (easy reopen while scrolling).
    if (btnFab) {
      btnFab.hidden = open;
      setExpanded(btnFab, open);
    }

    if (typeof lucide !== "undefined" && lucide.createIcons) {
      try {
        lucide.createIcons();
      } catch (e) {}
    }
  }

  function setOpen(next) {
    open = !!next;
    lsSet(open ? "1" : "0");
    apply();
  }

  function toggle() {
    setOpen(!open);
  }

  if (btnClose) btnClose.addEventListener("click", function () { setOpen(false); });
  if (btnOpen) btnOpen.addEventListener("click", toggle);
  if (btnFab) btnFab.addEventListener("click", function () { setOpen(true); });
  if (backdrop) {
    backdrop.addEventListener("click", function () { setOpen(false); });
  }

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && open) setOpen(false);
  });

  // Close panel after jumping to a heading on narrow screens (more room to read).
  sidebar.addEventListener("click", function (e) {
    var a = e.target.closest("a[href^='#']");
    if (!a) return;
    if (!isWide()) setOpen(false);
  });

  // Re-apply backdrop when crossing breakpoint (panel stays open/closed).
  var mq = window.matchMedia("(min-width: " + WIDE + "px)");
  if (mq.addEventListener) {
    mq.addEventListener("change", apply);
  } else if (mq.addListener) {
    mq.addListener(apply);
  }

  // Active section highlight while scrolling.
  var links = sidebar.querySelectorAll(".toc-nav a[href^='#']");
  if (links.length && "IntersectionObserver" in window) {
    var map = {};
    links.forEach(function (a) {
      var id = decodeURIComponent(a.getAttribute("href").slice(1));
      map[id] = a;
    });
    var io = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (en) {
          if (!en.isIntersecting) return;
          links.forEach(function (l) {
            l.classList.remove("is-active");
          });
          var a = map[en.target.id];
          if (a) a.classList.add("is-active");
        });
      },
      { rootMargin: "-20% 0px -70% 0px", threshold: 0 }
    );
    Object.keys(map).forEach(function (id) {
      var el = document.getElementById(id);
      if (el) io.observe(el);
    });
  }

  apply();
})();
