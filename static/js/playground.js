/**
 * Admin LLM playground — POST /admin/ai/chat/stream (SSE), render markdown live.
 * Expects marked + DOMPurify on window (CDN).
 */
(function () {
  "use strict";

  var sendBtn = document.getElementById("pg-send");
  var stopBtn = document.getElementById("pg-stop");
  var clearBtn = document.getElementById("pg-clear");
  var msgEl = document.getElementById("pg-message");
  var sysEl = document.getElementById("pg-system");
  var outEl = document.getElementById("pg-output");
  var rawEl = document.getElementById("pg-raw");
  var statusEl = document.getElementById("pg-status");
  var showRaw = document.getElementById("pg-show-raw");
  if (!sendBtn || !msgEl || !outEl) return;

  var abort = null;
  var buf = "";

  function csrfToken() {
    var el = document.querySelector('input[name="_csrf"]');
    return el ? el.value : "";
  }

  function setStatus(msg, isError) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.classList.toggle("ai-seo-error", !!isError);
  }

  function render() {
    if (rawEl) rawEl.textContent = buf;
    if (typeof marked !== "undefined" && typeof DOMPurify !== "undefined") {
      try {
        outEl.innerHTML = DOMPurify.sanitize(marked.parse(buf || ""));
      } catch (e) {
        outEl.textContent = buf;
      }
    } else {
      outEl.textContent = buf;
    }
    outEl.scrollTop = outEl.scrollHeight;
  }

  function setRunning(on) {
    sendBtn.disabled = on;
    if (stopBtn) stopBtn.disabled = !on;
  }

  function stop() {
    if (abort) {
      abort.abort();
      abort = null;
    }
    setRunning(false);
    setStatus("已停止", false);
  }

  async function send() {
    var message = (msgEl.value || "").trim();
    if (!message) {
      setStatus("請輸入 message", true);
      return;
    }
    buf = "";
    render();
    setRunning(true);
    setStatus("串流中…", false);
    abort = new AbortController();

    try {
      var res = await fetch("/admin/ai/chat/stream", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream",
          "X-Csrf-Token": csrfToken(),
        },
        credentials: "same-origin",
        signal: abort.signal,
        body: JSON.stringify({
          message: message,
          system: sysEl ? sysEl.value : "",
        }),
      });

      if (!res.ok) {
        var errText = await res.text();
        try {
          var j = JSON.parse(errText);
          throw new Error(j.error || res.statusText);
        } catch (e) {
          if (e.message && e.message !== errText) throw e;
          throw new Error(errText || res.statusText);
        }
      }

      var reader = res.body.getReader();
      var decoder = new TextDecoder();
      var lineBuf = "";

      while (true) {
        var chunk = await reader.read();
        if (chunk.done) break;
        lineBuf += decoder.decode(chunk.value, { stream: true });
        var parts = lineBuf.split("\n");
        lineBuf = parts.pop() || "";
        for (var i = 0; i < parts.length; i++) {
          var line = parts[i].replace(/\r$/, "");
          if (!line.startsWith("data:")) continue;
          var data = line.slice(5).trim();
          if (data === "[DONE]") {
            setStatus("完成", false);
            setRunning(false);
            abort = null;
            return;
          }
          try {
            var obj = JSON.parse(data);
            if (obj.error) {
              setStatus(obj.error, true);
              continue;
            }
            if (obj.delta) {
              buf += obj.delta;
              render();
            }
          } catch (e) {
            // ignore non-json lines
          }
        }
      }
      setStatus("完成", false);
    } catch (err) {
      if (err && err.name === "AbortError") {
        setStatus("已停止", false);
      } else {
        setStatus(String(err && err.message ? err.message : err), true);
      }
    } finally {
      setRunning(false);
      abort = null;
    }
  }

  sendBtn.addEventListener("click", send);
  if (stopBtn) stopBtn.addEventListener("click", stop);
  if (clearBtn) {
    clearBtn.addEventListener("click", function () {
      buf = "";
      render();
      setStatus("", false);
    });
  }
  if (showRaw && rawEl) {
    showRaw.addEventListener("change", function () {
      rawEl.hidden = !showRaw.checked;
    });
  }
  msgEl.addEventListener("keydown", function (e) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      send();
    }
  });
})();
