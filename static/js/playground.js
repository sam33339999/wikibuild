/**
 * LLM Streaming Playground — multi-turn chat via POST /admin/ai/chat/stream (SSE).
 * Renders assistant turns with marked + DOMPurify (CDN).
 */
(function () {
  "use strict";

  var sendBtn = document.getElementById("pg-send");
  var stopBtn = document.getElementById("pg-stop");
  var clearBtn = document.getElementById("pg-clear");
  var msgEl = document.getElementById("pg-message");
  var sysEl = document.getElementById("pg-system");
  var transcript = document.getElementById("pg-transcript");
  var outEl = document.getElementById("pg-output");
  var rawEl = document.getElementById("pg-raw");
  var statusEl = document.getElementById("pg-status");
  var showRaw = document.getElementById("pg-show-raw");
  if (!sendBtn || !msgEl || !transcript) return;

  /** @type {{role:string, content:string}[]} */
  var history = [];
  var abort = null;
  var streamBuf = "";
  var streamNode = null;

  function csrfToken() {
    var el = document.querySelector('input[name="_csrf"]');
    return el ? el.value : "";
  }

  function setStatus(msg, isError) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.classList.toggle("ai-seo-error", !!isError);
  }

  function mdHTML(text) {
    if (typeof marked !== "undefined" && typeof DOMPurify !== "undefined") {
      try {
        return DOMPurify.sanitize(marked.parse(text || ""));
      } catch (e) {
        /* fall through */
      }
    }
    var d = document.createElement("div");
    d.textContent = text || "";
    return d.innerHTML;
  }

  function appendTurn(role, content, streaming) {
    var wrap = document.createElement("div");
    wrap.className = "pg-turn pg-turn-" + role + (streaming ? " is-streaming" : "");
    var label = document.createElement("div");
    label.className = "pg-turn-role meta";
    label.textContent = role === "user" ? "You" : "Assistant";
    var body = document.createElement("div");
    body.className = "pg-turn-body content";
    if (role === "user") {
      body.textContent = content;
    } else {
      body.innerHTML = mdHTML(content);
    }
    wrap.appendChild(label);
    wrap.appendChild(body);
    transcript.appendChild(wrap);
    transcript.scrollTop = transcript.scrollHeight;
    return { wrap: wrap, body: body };
  }

  function updateStream(content) {
    if (!streamNode) return;
    streamNode.body.innerHTML = mdHTML(content);
    if (rawEl) rawEl.textContent = content;
    transcript.scrollTop = transcript.scrollHeight;
  }

  function setRunning(on) {
    sendBtn.disabled = on;
    if (stopBtn) stopBtn.disabled = !on;
    if (msgEl) msgEl.disabled = on;
  }

  function stop() {
    if (abort) {
      abort.abort();
      abort = null;
    }
    setRunning(false);
    setStatus("已停止", false);
    if (streamNode) {
      streamNode.wrap.classList.remove("is-streaming");
      if (streamBuf) {
        history.push({ role: "assistant", content: streamBuf });
      }
      streamNode = null;
      streamBuf = "";
    }
  }

  async function send() {
    var message = (msgEl.value || "").trim();
    if (!message) {
      setStatus("請輸入 message", true);
      return;
    }
    if (abort) return;

    appendTurn("user", message, false);
    history.push({ role: "user", content: message });
    msgEl.value = "";

    streamBuf = "";
    streamNode = appendTurn("assistant", "", true);
    setRunning(true);
    setStatus("串流中…", false);
    abort = new AbortController();

    // Prior turns only (exclude the user message we just appended — sent as message).
    var prior = history.slice(0, -1);

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
          messages: prior,
        }),
      });

      if (!res.ok) {
        var errText = await res.text();
        var errMsg = errText;
        try {
          var j = JSON.parse(errText);
          errMsg = j.error || res.statusText;
        } catch (e) {}
        throw new Error(errMsg || res.statusText);
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
            finishOK();
            return;
          }
          try {
            var obj = JSON.parse(data);
            if (obj.error) {
              setStatus(obj.error, true);
              continue;
            }
            if (obj.delta) {
              streamBuf += obj.delta;
              updateStream(streamBuf);
            }
          } catch (e) {
            /* ignore */
          }
        }
      }
      finishOK();
    } catch (err) {
      if (err && err.name === "AbortError") {
        setStatus("已停止", false);
      } else {
        setStatus(String(err && err.message ? err.message : err), true);
        if (streamNode && !streamBuf) {
          streamNode.body.textContent = "(錯誤)";
        }
      }
    } finally {
      setRunning(false);
      abort = null;
      if (streamNode) {
        streamNode.wrap.classList.remove("is-streaming");
        streamNode = null;
      }
    }
  }

  function finishOK() {
    setStatus("完成", false);
    if (streamBuf) {
      history.push({ role: "assistant", content: streamBuf });
    }
    if (streamNode) {
      streamNode.wrap.classList.remove("is-streaming");
      streamNode = null;
    }
    streamBuf = "";
    setRunning(false);
    abort = null;
  }

  sendBtn.addEventListener("click", send);
  if (stopBtn) stopBtn.addEventListener("click", stop);
  if (clearBtn) {
    clearBtn.addEventListener("click", function () {
      if (abort) stop();
      history = [];
      streamBuf = "";
      streamNode = null;
      transcript.innerHTML = "";
      if (rawEl) rawEl.textContent = "";
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
