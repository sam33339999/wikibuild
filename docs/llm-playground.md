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
| **Article tools** | 與 MCP **同一套** 6 個工具（tool use 迴圈） |
| IME | 中文候選確認 Enter **不會**送出訊息 |
| 停止 | AbortController 中斷 |
| 清除 | 清空對話歷史 |
| Rate limit | 預設 30 次 stream / 分鐘（process 內） |

### Article tools（勾選後）

與 [`integrations-mcp.md`](./integrations-mcp.md) 相同：

`list_articles` · `get_article` · `create_article` · `update_article` · `set_article_status` · `set_article_visibility`

- create 預設 **draft + private**
- 工具由 `internal/mcp.Tools` 執行（與 `wikibuild mcp` 同源）
- 無 tools 時：純 SSE `stream:true` 文字串流
- 有 tools 時：非 stream chat + tool loop，最終文字以 `delta` 事件推送

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
  "message": "latest user turn",
  "tools": true
}
```

SSE 事件：

```
data: {"type":"tool_call","id":"…","name":"list_articles","arguments":"{…}"}

data: {"type":"tool_result","id":"…","name":"list_articles","result":"…"}

data: {"delta":"Hel"}

data: {"delta":"lo"}

data: [DONE]
```

錯誤時可能先回 JSON（非 stream），或 stream 內 `{"error":"…"}`。

## 實作

| 路徑 | 內容 |
|------|------|
| `internal/llm/stream.go` | OpenAI `stream:true` + SSE 解析 |
| `internal/llm/chat.go` | `BuildChatMessages` |
| `internal/llm/tools_def.go` | OpenAI tools[] schema |
| `internal/llm/agent.go` | tool-call 迴圈 `RunAgent` |
| `internal/mcp/execute.go` | 依 tool name 執行（MCP 共用） |
| `internal/handler/playground.go` | 頁面 + stream / agent |
| `static/js/playground.js` | 多輪 UI、IME、tool 卡片 |

測試：`go test ./internal/llm/ ./internal/mcp/ ./internal/handler/ -run 'Chat|Stream|Playground|Agent|Execute|Tool' -count=1`。
