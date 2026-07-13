/**
 * Vditor editor for admin article form (CDN script, no npm / no ESM graph).
 * Modes: IR (Typora-like instant render) + optional WYSIWYG / source.
 * Syncs markdown into #body on submit; paste/drop images → /admin/media.
 *
 * Expects global `Vditor` from:
 *   https://cdn.jsdelivr.net/npm/vditor@3.10.9/dist/index.min.js
 */
(function () {
  var form = document.getElementById("article-form");
  var textarea = document.getElementById("body");
  var host = document.getElementById("md-editor");
  var statusEl = document.getElementById("media-status");
  var csrfInput = document.querySelector('input[name="_csrf"]');
  var modeBtn = document.getElementById("editor-toggle-source");

  if (!form || !textarea || !host) return;

  if (typeof Vditor === "undefined") {
    showFallback("編輯器腳本未載入，已改用純文字。");
    return;
  }

  setStatus("正在初始化編輯器…");
  host.classList.add("is-loading");

  var initial = textarea.value || "";
  var vditor = null;

  // Resolve light/dark for Vditor chrome.
  function themeName() {
    var mode = "auto";
    try {
      mode = localStorage.getItem("wikibuild-theme") || "auto";
    } catch (e) {}
    if (mode === "dark") return "dark";
    if (mode === "light") return "classic";
    if (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) {
      return "dark";
    }
    return "classic";
  }

  try {
    vditor = new Vditor("md-editor", {
      height: 480,
      mode: "ir", // Instant Rendering — type markdown, see formatted result live
      value: initial,
      theme: themeName(),
      icon: "ant",
      cache: { enable: false },
      toolbarConfig: { pin: true },
      toolbar: [
        "emoji",
        "headings",
        "bold",
        "italic",
        "strike",
        "link",
        "|",
        "list",
        "ordered-list",
        "check",
        "outdent",
        "indent",
        "|",
        "quote",
        "line",
        "code",
        "inline-code",
        "insert-before",
        "insert-after",
        "|",
        "upload",
        "table",
        "|",
        "undo",
        "redo",
        "|",
        "fullscreen",
        "edit-mode",
        "outline",
        "preview",
      ],
      preview: {
        theme: { current: themeName() === "dark" ? "dark" : "light" },
        hljs: { style: themeName() === "dark" ? "native" : "github" },
      },
      // Custom image upload → our /admin/media endpoint.
      upload: {
        accept: "image/*",
        multiple: false,
        handler: function (files) {
          if (!files || !files.length) return null;
          uploadFile(files[0]);
          return null; // we handle insert ourselves
        },
      },
      after: function () {
        host.classList.remove("is-loading");
        hideTextarea();
        setStatus("即時渲染（IR）已就緒 — 可用工具列，或切換「所見即所得 / 原始碼」。");
        if (modeBtn) modeBtn.hidden = false;
      },
      input: function () {
        // Keep textarea roughly in sync for safety (submit still reads getValue).
        try {
          textarea.value = vditor.getValue();
        } catch (e) {}
      },
    });
  } catch (err) {
    console.error(err);
    host.classList.remove("is-loading");
    showFallback("編輯器初始化失敗：" + (err && err.message ? err.message : err));
    return;
  }

  form.addEventListener("submit", function () {
    if (vditor) {
      try {
        textarea.value = vditor.getValue();
      } catch (e) {}
    }
  });

  // Cycle IR → wysiwyg → sv (source) for the toolbar button label helper.
  if (modeBtn) {
    modeBtn.addEventListener("click", function () {
      if (!vditor) return;
      var cur = vditor.getCurrentMode ? vditor.getCurrentMode() : "ir";
      var next = cur === "ir" ? "wysiwyg" : cur === "wysiwyg" ? "sv" : "ir";
      vditor.setCurrentMode(next);
      modeBtn.textContent =
        next === "ir" ? "模式：即時渲染" : next === "wysiwyg" ? "模式：所見即所得" : "模式：原始碼";
      modeBtn.setAttribute("data-mode", next);
    });
  }

  document.addEventListener("wikibuild:theme", function () {
    if (!vditor || !vditor.setTheme) return;
    var t = themeName();
    vditor.setTheme(t, t === "dark" ? "dark" : "light", t === "dark" ? "native" : "github");
  });

  // Extra paste/drop on host (in case toolbar upload isn't used).
  host.addEventListener("paste", function (e) {
    var items = e.clipboardData && e.clipboardData.items;
    if (!items) return;
    for (var i = 0; i < items.length; i++) {
      if (items[i].type.indexOf("image/") === 0) {
        e.preventDefault();
        uploadFile(items[i].getAsFile());
        return;
      }
    }
  });

  function uploadFile(file) {
    if (!file || !csrfInput) {
      setStatus("無法上傳（缺 CSRF）");
      return;
    }
    setStatus("上傳中…");
    var fd = new FormData();
    fd.append("file", file);
    fd.append("_csrf", csrfInput.value);
    fetch("/admin/media", {
      method: "POST",
      headers: { "X-Csrf-Token": csrfInput.value },
      body: fd,
      credentials: "same-origin",
    })
      .then(function (r) {
        if (!r.ok) {
          return r.json().then(function (j) {
            throw new Error(j.error || r.statusText);
          });
        }
        return r.json();
      })
      .then(function (j) {
        var alt = (file.name || "image").replace(/\.[^.]+$/, "");
        var md = "![" + alt + "](" + j.url + ")";
        if (vditor && vditor.insertValue) {
          vditor.insertValue(md);
        } else if (vditor) {
          vditor.setValue(vditor.getValue() + "\n\n" + md + "\n\n");
        }
        setStatus("已插入 " + j.url);
      })
      .catch(function (e) {
        setStatus("上傳失敗：" + (e.message || e));
      });
  }

  function hideTextarea() {
    textarea.classList.add("sr-only");
    textarea.setAttribute("tabindex", "-1");
    textarea.setAttribute("aria-hidden", "true");
  }

  function showFallback(msg) {
    host.hidden = true;
    textarea.classList.remove("sr-only");
    textarea.hidden = false;
    textarea.removeAttribute("tabindex");
    textarea.removeAttribute("aria-hidden");
    if (modeBtn) modeBtn.hidden = true;
    setStatus(msg);
  }

  function setStatus(msg) {
    if (statusEl) statusEl.textContent = msg || "";
  }

  window.__wikibuildEditor = function () {
    return vditor;
  };
})();
