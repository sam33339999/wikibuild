/**
 * S3a — writing-time site search panel.
 * Queries GET /admin/api/articles/search and inserts [[slug]] into Vditor
 * (or the #body textarea fallback).
 */
(function () {
  "use strict";

  var panel = document.getElementById("editor-search");
  if (!panel) return;

  var input = document.getElementById("editor-search-q");
  var list = document.getElementById("editor-search-results");
  var status = document.getElementById("editor-search-status");
  var endpoint = panel.getAttribute("data-endpoint") || "/admin/api/articles/search";
  var excludeId = panel.getAttribute("data-exclude-id") || "0";
  var timer = null;
  var lastQuery = "";

  function setStatus(msg) {
    if (status) status.textContent = msg || "";
  }

  function insertWikilink(slug) {
    var md = "[[" + slug + "]]";
    if (typeof window.__wikibuildEditor === "function") {
      var v = window.__wikibuildEditor();
      if (v && typeof v.insertValue === "function") {
        v.insertValue(md);
        setStatus("已插入 [[" + slug + "]]");
        return;
      }
      if (v && typeof v.getValue === "function" && typeof v.setValue === "function") {
        v.setValue(v.getValue() + md);
        setStatus("已插入 [[" + slug + "]]");
        return;
      }
    }
    var ta = document.getElementById("body");
    if (ta) {
      var start = ta.selectionStart || ta.value.length;
      var end = ta.selectionEnd || start;
      ta.value = ta.value.slice(0, start) + md + ta.value.slice(end);
      ta.focus();
      var pos = start + md.length;
      if (ta.setSelectionRange) ta.setSelectionRange(pos, pos);
      setStatus("已插入 [[" + slug + "]]");
      return;
    }
    setStatus("無法插入：編輯器未就緒");
  }

  function render(items) {
    list.innerHTML = "";
    if (!items || !items.length) {
      setStatus(lastQuery ? "找不到相符文章" : "");
      return;
    }
    setStatus(items.length + " 筆結果（點選插入 wikilink）");
    items.forEach(function (it) {
      var li = document.createElement("li");
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "editor-search-hit";
      btn.innerHTML =
        '<span class="editor-search-hit-title"></span>' +
        '<span class="editor-search-hit-meta meta"></span>';
      btn.querySelector(".editor-search-hit-title").textContent = it.title || it.slug;
      btn.querySelector(".editor-search-hit-meta").textContent =
        "[[" + it.slug + "]] · " + (it.status || "") + " · " + (it.visibility || "");
      btn.addEventListener("click", function () {
        insertWikilink(it.slug);
      });
      li.appendChild(btn);
      list.appendChild(li);
    });
  }

  function search(q) {
    lastQuery = q;
    if (!q) {
      list.innerHTML = "";
      setStatus("");
      return;
    }
    setStatus("搜尋中…");
    var url =
      endpoint +
      "?q=" +
      encodeURIComponent(q) +
      "&exclude_id=" +
      encodeURIComponent(excludeId);
    fetch(url, { credentials: "same-origin", headers: { Accept: "application/json" } })
      .then(function (res) {
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(render)
      .catch(function (err) {
        list.innerHTML = "";
        setStatus("搜尋失敗：" + (err && err.message ? err.message : err));
      });
  }

  if (input) {
    input.addEventListener("input", function () {
      var q = (input.value || "").trim();
      if (timer) clearTimeout(timer);
      timer = setTimeout(function () {
        search(q);
      }, 220);
    });
    input.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        input.value = "";
        search("");
        input.blur();
      }
    });
  }
})();
