/**
 * Initialize Lucide icons (data-lucide attributes).
 * Expects global lucide from UMD build.
 */
(function () {
  function run() {
    if (typeof lucide === "undefined" || !lucide.createIcons) {
      console.warn("lucide not loaded");
      return;
    }
    lucide.createIcons({
      attrs: {
        "stroke-width": 2,
        class: "icon",
      },
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", run);
  } else {
    run();
  }

  // Re-run after theme toggle in case DOM was mutated (usually not needed).
  document.addEventListener("wikibuild:theme", function () {
    // Icons are SVG already; nothing to do. Keep hook for future.
  });
})();
