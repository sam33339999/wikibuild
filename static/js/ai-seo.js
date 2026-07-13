/**
 * Admin AI SEO generate + OG image helper.
 * Does not save the article — author must click 儲存.
 */
(function () {
  "use strict";

  function csrfToken() {
    var el = document.querySelector('input[name="_csrf"]');
    return el ? el.value : "";
  }

  function formBody() {
    if (typeof window.__wikibuildEditor === "function") {
      var v = window.__wikibuildEditor();
      if (v && typeof v.getValue === "function") {
        return v.getValue();
      }
    }
    var ta = document.getElementById("body") || document.querySelector('textarea[name="body"]');
    return ta ? ta.value : "";
  }

  function formTitle() {
    var el = document.querySelector('input[name="title"]');
    return el ? el.value : "";
  }

  function setStatus(el, msg, isError) {
    if (!el) return;
    el.textContent = msg || "";
    el.classList.toggle("ai-seo-error", !!isError);
  }

  function confirmOverwrite() {
    var sum = document.querySelector('textarea[name="summary"]');
    var meta = document.querySelector('textarea[name="meta_description"]');
    var has =
      (sum && sum.value.trim()) ||
      (meta && meta.value.trim());
    if (!has) return true;
    return window.confirm("摘要或 Meta description 已有內容，確定要用 AI 結果覆寫？");
  }

  function fillFields(data) {
    var sum = document.querySelector('textarea[name="summary"]');
    var meta = document.querySelector('textarea[name="meta_description"]');
    if (sum && typeof data.summary === "string") sum.value = data.summary;
    if (meta && typeof data.meta_description === "string") meta.value = data.meta_description;

    var box = document.getElementById("ai-seo-outline");
    var list = document.getElementById("ai-seo-outline-list");
    if (!box || !list) return;
    list.innerHTML = "";
    var outline = Array.isArray(data.outline) ? data.outline : [];
    if (outline.length === 0) {
      box.hidden = true;
      return;
    }
    outline.forEach(function (item) {
      var li = document.createElement("li");
      li.textContent = String(item);
      list.appendChild(li);
    });
    box.hidden = false;
  }

  function endpointFor(btn) {
    var id = btn.getAttribute("data-article-id") || "0";
    if (id && id !== "0") {
      return "/admin/" + id + "/ai/seo";
    }
    return btn.getAttribute("data-endpoint") || "/admin/ai/seo";
  }

  function postForm(url, params) {
    return fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Csrf-Token": csrfToken(),
      },
      body: params.toString(),
      credentials: "same-origin",
    }).then(function (res) {
      return res.json().then(function (data) {
        return { ok: res.ok, status: res.status, data: data };
      });
    });
  }

  function initSEO() {
    var btn = document.getElementById("ai-seo-btn");
    if (!btn) return;
    var status = document.getElementById("ai-seo-status");
    var articleId = btn.getAttribute("data-article-id") || "0";

    btn.addEventListener("click", function () {
      var body = formBody();
      // Markdown needs body in form; html_upload with saved id can omit body
      // (server loads & strips HTML from disk).
      if (!String(body).trim() && (!articleId || articleId === "0")) {
        setStatus(status, "請先寫入正文再產生。", true);
        return;
      }
      if (!confirmOverwrite()) return;

      btn.disabled = true;
      setStatus(status, "產生中…", false);

      var params = new URLSearchParams();
      params.set("_csrf", csrfToken());
      params.set("title", formTitle());
      if (String(body).trim()) params.set("body", body);

      postForm(endpointFor(btn), params)
        .then(function (r) {
          if (!r.ok) {
            setStatus(status, (r.data && r.data.error) || "錯誤 " + r.status, true);
            return;
          }
          fillFields(r.data);
          setStatus(status, "已填入表單（尚未儲存）", false);
        })
        .catch(function (err) {
          setStatus(status, String(err && err.message ? err.message : err), true);
        })
        .finally(function () {
          btn.disabled = false;
        });
    });
  }

  function initOG() {
    var btn = document.getElementById("ai-og-btn");
    if (!btn) return;
    var status = document.getElementById("ai-og-status");

    btn.addEventListener("click", function () {
      var og = document.querySelector('input[name="og_image_url"]');
      if (og && og.value.trim()) {
        if (!window.confirm("OG 圖 URL 已有值，確定覆寫為新產生的圖？")) return;
      }
      btn.disabled = true;
      setStatus(status, "產生圖片中…", false);
      var params = new URLSearchParams();
      params.set("_csrf", csrfToken());
      params.set("title", formTitle());
      var ep = btn.getAttribute("data-endpoint");
      postForm(ep, params)
        .then(function (r) {
          if (!r.ok) {
            setStatus(status, (r.data && r.data.error) || "錯誤 " + r.status, true);
            return;
          }
          if (og && r.data && r.data.url) {
            og.value = r.data.url;
          }
          setStatus(status, "已填入 OG URL（尚未儲存）", false);
        })
        .catch(function (err) {
          setStatus(status, String(err && err.message ? err.message : err), true);
        })
        .finally(function () {
          btn.disabled = false;
        });
    });
  }

  function init() {
    initSEO();
    initOG();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
