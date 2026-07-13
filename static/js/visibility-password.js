/**
 * Show/highlight the per-article protected password fields when visibility
 * is set to "protected".
 */
(function () {
  function sync() {
    var sel = document.getElementById("visibility");
    var box = document.getElementById("protected-password-fields");
    if (!sel || !box) return;
    var on = sel.value === "protected";
    box.hidden = !on;
    box.classList.toggle("is-active", on);
    var input = box.querySelector('input[name="password"]');
    if (input) {
      if (on) input.removeAttribute("tabindex");
      else input.setAttribute("tabindex", "-1");
    }
  }
  var sel = document.getElementById("visibility");
  if (!sel) return;
  sel.addEventListener("change", sync);
  sync();
})();
