/**
 * LLM Streaming Playground — multi-turn chat via POST /admin/ai/chat/stream (SSE).
 * Live phase bar + elapsed timer so long tool rounds stay visible.
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
  var rawEl = document.getElementById("pg-raw");
  var statusEl = document.getElementById("pg-status");
  var showRaw = document.getElementById("pg-show-raw");
  var toolsEl = document.getElementById("pg-tools");
  var liveEl = document.getElementById("pg-live");
  var livePhase = document.getElementById("pg-live-phase");
  var liveElapsed = document.getElementById("pg-live-elapsed");
  var liveDetail = document.getElementById("pg-live-detail");
  if (!sendBtn || !msgEl || !transcript) return;

  /** @type {{role:string, content:string}[]} */
  var history = [];
  var abort = null;
  var streamBuf = "";
  var streamNode = null;
  var runStart = 0;
  var tickTimer = null;
  var lastPhase = "";
  var logSeq = 0;

  function csrfToken() {
    var el = document.querySelector('input[name="_csrf"]');
    return el ? el.value : "";
  }

  function fmtElapsed(ms) {
    var s = ms / 1000;
    if (s < 60) return s.toFixed(1) + "s";
    var m = Math.floor(s / 60);
    var r = (s % 60).toFixed(0);
    return m + "m " + r + "s";
  }

  function setStatus(msg, isError) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.classList.toggle("ai-seo-error", !!isError);
  }

  function setLive(phase, detail, running) {
    lastPhase = phase || lastPhase;
    if (liveEl) {
      liveEl.hidden = !running && !phase;
      if (running) liveEl.classList.add("is-active");
      else liveEl.classList.remove("is-active");
    }
    if (livePhase && phase) livePhase.textContent = phase;
    if (liveDetail && detail !== undefined) liveDetail.textContent = detail || "";
    if (liveElapsed && runStart) {
      liveElapsed.textContent = fmtElapsed(Date.now() - runStart);
    }
    if (phase) setStatus(phase + (detail ? " — " + detail : ""), false);
  }

  function startTicker() {
    stopTicker();
    runStart = Date.now();
    tickTimer = setInterval(function () {
      if (liveElapsed && runStart) {
        liveElapsed.textContent = fmtElapsed(Date.now() - runStart);
      }
    }, 200);
  }

  function stopTicker() {
    if (tickTimer) {
      clearInterval(tickTimer);
      tickTimer = null;
    }
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
    label.textContent =
      role === "user" ? "You" : role === "tool" ? "Tool" : "Assistant";
    var body = document.createElement("div");
    body.className = "pg-turn-body content";
    if (role === "user" || role === "tool") {
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

  function appendProgress(text) {
    logSeq++;
    var wrap = document.createElement("div");
    wrap.className = "pg-turn pg-turn-progress";
    wrap.dataset.seq = String(logSeq);
    var body = document.createElement("div");
    body.className = "pg-turn-body meta";
    var t = document.createElement("span");
    t.className = "pg-progress-time";
    t.textContent = runStart ? fmtElapsed(Date.now() - runStart) : "";
    body.appendChild(t);
    body.appendChild(document.createTextNode(" · " + text));
    wrap.appendChild(body);
    transcript.appendChild(wrap);
    transcript.scrollTop = transcript.scrollHeight;
  }

  function appendToolCard(kind, name, detail) {
    var wrap = document.createElement("div");
    wrap.className = "pg-turn pg-turn-tool pg-tool-" + kind;
    var label = document.createElement("div");
    label.className = "pg-turn-role meta";
    label.textContent = kind === "tool_call" ? "🔧 Tool call" : "✅ Tool result";
    var body = document.createElement("div");
    body.className = "pg-turn-body";
    var title = document.createElement("code");
    title.textContent = name || "";
    body.appendChild(title);
    if (runStart) {
      var tm = document.createElement("span");
      tm.className = "meta pg-tool-when";
      tm.textContent = " @ " + fmtElapsed(Date.now() - runStart);
      body.appendChild(tm);
    }
    var pre = document.createElement("pre");
    pre.className = "pg-tool-detail";
    pre.textContent = detail || "";
    body.appendChild(pre);
    wrap.appendChild(label);
    wrap.appendChild(body);
    transcript.appendChild(wrap);
    transcript.scrollTop = transcript.scrollHeight;
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
    if (!on) {
      stopTicker();
      if (liveEl) liveEl.classList.remove("is-active");
    }
  }

  function stop() {
    if (abort) {
      abort.abort();
      abort = null;
    }
    setRunning(false);
    setLive("已停止", "", false);
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

    var useTools = !!(toolsEl && toolsEl.checked);

    appendTurn("user", message, false);
    history.push({ role: "user", content: message });
    msgEl.value = "";

    streamBuf = "";
    // Don't open empty assistant bubble until first delta (avoids blank card while tools run).
    streamNode = null;
    setRunning(true);
    startTicker();
    setLive(
      useTools ? "啟動（tools 模式）" : "啟動（純 stream）",
      "連線中…",
      true
    );
    appendProgress(useTools ? "開始 · tools 開啟" : "開始 · 純文字串流");
    abort = new AbortController();

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
          tools: useTools,
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

      setLive("已連線", "等待伺服器事件…", true);
      appendProgress("SSE 已連線");

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
          if (!line || line.charAt(0) === ":") {
            // keepalive comment — still refresh "alive" feel
            if (line.indexOf("keepalive") >= 0) {
              setLive(lastPhase || "等待中", "heartbeat（模型仍在處理）", true);
            }
            continue;
          }
          if (!line.startsWith("data:")) continue;
          var data = line.slice(5).trim();
          if (data === "[DONE]") {
            finishOK();
            return;
          }
          try {
            var obj = JSON.parse(data);
            if (obj.error) {
              setLive("錯誤", obj.error, false);
              setStatus(obj.error, true);
              appendProgress("錯誤: " + obj.error);
              continue;
            }
            if (obj.type === "status") {
              var sm = obj.message || "…";
              setLive(sm, "", true);
              appendProgress(sm);
              continue;
            }
            if (obj.type === "tool_call") {
              setLive("呼叫工具", obj.name || "", true);
              appendProgress("→ tool_call " + (obj.name || ""));
              appendToolCard("tool_call", obj.name, obj.arguments || "");
              continue;
            }
            if (obj.type === "tool_result") {
              setLive("工具完成", obj.name || "", true);
              appendProgress("← tool_result " + (obj.name || ""));
              var resText = obj.result || "";
              if (resText.length > 2000) resText = resText.slice(0, 2000) + "…";
              appendToolCard("tool_result", obj.name, resText);
              continue;
            }
            if (obj.delta) {
              if (!streamNode) {
                streamNode = appendTurn("assistant", "", true);
                appendProgress("開始輸出文字");
              }
              streamBuf += obj.delta;
              updateStream(streamBuf);
              setLive(
                "串流輸出中",
                streamBuf.length + " chars",
                true
              );
            }
          } catch (e) {
            /* ignore */
          }
        }
      }
      finishOK();
    } catch (err) {
      if (err && err.name === "AbortError") {
        setLive("已停止", "", false);
        setStatus("已停止", false);
        appendProgress("使用者停止");
      } else {
        var em = String(err && err.message ? err.message : err);
        setLive("失敗", em, false);
        setStatus(em, true);
        appendProgress("失敗: " + em);
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
    var el = runStart ? fmtElapsed(Date.now() - runStart) : "";
    setLive("完成", el, false);
    setStatus("完成" + (el ? " · " + el : ""), false);
    appendProgress("完成" + (el ? " · " + el : ""));
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
      if (liveEl) liveEl.hidden = true;
      if (livePhase) livePhase.textContent = "待命";
      if (liveDetail) liveDetail.textContent = "";
      if (liveElapsed) liveElapsed.textContent = "0.0s";
    });
  }
  if (showRaw && rawEl) {
    showRaw.addEventListener("change", function () {
      rawEl.hidden = !showRaw.checked;
    });
  }
  // IME: do not submit while composing
  var composing = false;
  msgEl.addEventListener("compositionstart", function () {
    composing = true;
  });
  msgEl.addEventListener("compositionend", function () {
    composing = false;
  });
  msgEl.addEventListener("keydown", function (e) {
    if (e.key !== "Enter" || e.shiftKey) return;
    if (composing || e.isComposing || e.keyCode === 229) return;
    e.preventDefault();
    send();
  });
})();
