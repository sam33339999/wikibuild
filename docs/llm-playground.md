# LLM Streaming Playground

後台 **多輪對話 + SSE 串流** 試玩頁，不會寫入文章。

## 開啟

1. `.env` 設定：

```bash
WIKIBUILD_LLM_BASE_URL=https://api.deepseek.com   # 或 /v1 視 provider
WIKIBUILD_LLM_API_KEY=...
WIKIBUILD_LLM_MODEL=...
```

2. 重啟 `wikibuild`。
3. 登入後台 → **LLM Streaming** → `/admin/playground`。

未設定 LLM 時頁面會顯示設定說明，不顯示表單。

## 功能

| 功能 | 說明 |
|------|------|
| System | 可選 system prompt，每輪都會帶上 |
| 多輪 | 前端保留 user/assistant 歷史，一併 POST |
| Streaming | `POST /admin/ai/chat/stream` → `text/event-stream` |
| Markdown | 串流中以 marked + DOMPurify 渲染 |
| 停止 | AbortController 中斷 |
| 清除 | 清空對話歷史 |
| Rate limit | 預設 30 次 stream / 分鐘（process 內） |

## API

```http
POST /admin/ai/chat/stream
Content-Type: application/json
X-Csrf-Token: …

{
  "system": "optional",
  "messages": [
    {"role": "user", "content": "…"},
    {"role": "assistant", "content": "…"}
  ],
  "message": "latest user turn"
}
```

SSE：

```
data: {"delta":"Hel"}

data: {"delta":"lo"}

data: [DONE]
```

錯誤時可能先回 JSON（非 stream），或 stream 內 `{"error":"…"}`。

## 實作

| 路徑 | 內容 |
|------|------|
| `internal/llm/stream.go` | OpenAI `stream:true` + SSE 解析 |
| `internal/llm/chat.go` | `BuildChatMessages`（歷史裁剪、角色檢查） |
| `internal/handler/playground.go` | 頁面 + stream |
| `static/js/playground.js` | 多輪 UI + 串流渲染 |
| `views/admin/playground.templ` | 頁面 |

測試：`go test ./internal/llm/ ./internal/handler/ -run 'Chat|Stream|Playground' -count=1`。
