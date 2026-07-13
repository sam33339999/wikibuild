# WikiBuild MCP（stdio）

透過 [Model Context Protocol](https://modelcontextprotocol.io) 讓外部代理（Cursor、Claude Desktop、Grok 等）**本機讀寫文章**，不必爬 admin HTML。

| 項目 | 說明 |
|------|------|
| 傳輸 | **stdio**（子行程 stdin/stdout） |
| 啟動 | `wikibuild mcp` |
| 認證 | 環境變數 `WIKIBUILD_MCP_TOKEN`（空 → **拒絕啟動**） |
| 資料 | 與網頁共用 PostgreSQL + `store.Repository` |

**不是** HTTP REST API，也**不要**把這個 process 暴露到公網。

---

## 前置條件

1. PostgreSQL 已 migrate（`make migrate-up`，含 SEO 等欄位）。
2. 應用設定可被 `config.Load` 通過（與跑 HTTP server 相同）。
3. 設定非空的 MCP token。

### `.env` 範例

```bash
DATABASE_URL=postgres://wikibuild:wikibuild@localhost:5432/wikibuild?sslmode=disable
WIKIBUILD_ADMIN_USER=admin
WIKIBUILD_ADMIN_PASS=changeme
WIKIBUILD_SESSION_SECRET=replace-with-at-least-16-chars
# 必填：空 token 會 fail closed
WIKIBUILD_MCP_TOKEN=  # 建議: openssl rand -hex 24
```

產生 token：

```bash
openssl rand -hex 24
# 寫入 .env 的 WIKIBUILD_MCP_TOKEN=
```

---

## 本機手動啟動

```bash
# 專案根目錄（會 godotenv 載入 .env）
go run ./cmd/wikibuild mcp

# 或先編譯
go build -o wikibuild ./cmd/wikibuild
./wikibuild mcp
```

- 成功時行程會**掛起等 MCP client**（正常現象）。
- **stdout = MCP 協定**，不要把 log 導到 stdout；除錯訊息在 stderr。
- token 為空時會立刻失敗，例如：  
  `WIKIBUILD_MCP_TOKEN is required to run MCP …`

自檢：

```bash
# 應失敗
WIKIBUILD_MCP_TOKEN= go run ./cmd/wikibuild mcp

# 應掛起（Ctrl+C 結束）— 確認 .env 已設 token 與 DATABASE_URL
go run ./cmd/wikibuild mcp
```

---

## 在 Cursor 設定

常見位置：`~/.cursor/mcp.json` 或專案 `.cursor/mcp.json`（依 Cursor 版本為準）。

### 方式 A：直接 `go run`（開發方便）

把路徑改成你的 repo 與真實密鑰：

```json
{
  "mcpServers": {
    "wikibuild": {
      "command": "go",
      "args": ["run", "./cmd/wikibuild", "mcp"],
      "cwd": "/absolute/path/to/wikibuild",
      "env": {
        "DATABASE_URL": "postgres://wikibuild:wikibuild@localhost:5432/wikibuild?sslmode=disable",
        "WIKIBUILD_ADMIN_USER": "admin",
        "WIKIBUILD_ADMIN_PASS": "changeme",
        "WIKIBUILD_SESSION_SECRET": "your-long-session-secret",
        "WIKIBUILD_MCP_TOKEN": "your-mcp-token"
      }
    }
  }
}
```

### 方式 B：編譯後的 binary（較穩）

```bash
go build -o /absolute/path/to/wikibuild/wikibuild ./cmd/wikibuild
```

```json
{
  "mcpServers": {
    "wikibuild": {
      "command": "/absolute/path/to/wikibuild/wikibuild",
      "args": ["mcp"],
      "env": {
        "DATABASE_URL": "postgres://wikibuild:wikibuild@localhost:5432/wikibuild?sslmode=disable",
        "WIKIBUILD_ADMIN_USER": "admin",
        "WIKIBUILD_ADMIN_PASS": "changeme",
        "WIKIBUILD_SESSION_SECRET": "your-long-session-secret",
        "WIKIBUILD_MCP_TOKEN": "your-mcp-token"
      }
    }
  }
}
```

重啟 Cursor 或重新載入 MCP 後，工具列表應出現 **wikibuild** 底下 6 個 tool。

聊天可試：「用 wikibuild MCP 列出最近的 draft 文章」。

---

## 在 Claude Desktop 設定

編輯 Claude Desktop 的 `claude_desktop_config.json`（macOS 約在  
`~/Library/Application Support/Claude/claude_desktop_config.json`），結構相同：

```json
{
  "mcpServers": {
    "wikibuild": {
      "command": "/absolute/path/to/wikibuild/wikibuild",
      "args": ["mcp"],
      "env": {
        "DATABASE_URL": "postgres://…",
        "WIKIBUILD_ADMIN_USER": "admin",
        "WIKIBUILD_ADMIN_PASS": "…",
        "WIKIBUILD_SESSION_SECRET": "…",
        "WIKIBUILD_MCP_TOKEN": "…"
      }
    }
  }
}
```

存檔後重啟 Claude Desktop。

---

## 工具一覽

| 工具 | 用途 | 主要參數 |
|------|------|----------|
| `list_articles` | 列表／篩選 | `status`（`draft` \| `published`）、`visibility`（`public` \| `protected` \| `private`）、`q`（標題／內文搜尋）、`limit`、`offset` |
| `get_article` | 取單篇（**含 body**） | `id` **或** `slug`（擇一） |
| `create_article` | 新建 Markdown 文章 | **必填** `slug`、`title`；可選見下表 |
| `update_article` | 依 id 部分更新 | **必填** `id`；其餘欄位有傳才改 |
| `set_article_status` | 草稿／發布 | **必填** `id`、`status`；可選 `publish_at`（RFC3339，僅草稿排程） |
| `set_article_visibility` | 可見性 | **必填** `id`、`visibility` |

### `create_article` 可選欄位

| 參數 | 說明 |
|------|------|
| `body` | Markdown 正文 |
| `tags` | 字串陣列 |
| `status` | 預設 **`draft`** |
| `visibility` | 預設 **`private`** |
| `seo_title` / `summary` / `meta_description` | SEO |
| `cover_image_url` / `og_image_url` | 封面／OG |
| `pinned` | 置頂 |
| `show_toc` | 目錄（預設 true） |

### 行為約定

- **建立預設 `draft` + `private`**，避免代理默默公開發布。
- 回應 **不含** 密碼 hash。
- `list_articles` **不含 body**（省 token）；全文請 `get_article`。
- 工具錯誤以 MCP tool error 文字回傳（如 `not found`、`duplicate slug`）。

### 建議代理工作流

```
list_articles(q=… / status=draft)
  → get_article(slug=… 或 id=…)
  → create_article(…) 或 update_article(id=…)
  → set_article_status(id=…, status=published)   # 若要發布
  → set_article_visibility(id=…, visibility=public)  # 若要公開
```

---

## 目前沒有的能力

MCP **不**提供（請用 admin 網頁或之後再擴充）：

- 刪除文章  
- 媒體上傳、`html_upload` zip 上版  
- AI 產生 SEO / 相關建議 / OG 圖（那些是 HTTP admin 功能）  
- 標籤管理、redirects、站台設定  

---

## 安全

| 點 | 建議 |
|----|------|
| 部署面 | 只在本機給 agent 起子行程；不要做成對外服務 |
| Token | 當本機開關與防呆；仍假設能執行 `wikibuild mcp` 的人可信 |
| 密鑰 | 勿把含真實密碼的 `mcp.json` 提交進 git |
| 發布 | 代理要公開文章需**明確**呼叫 status + visibility |

---

## 實作位置（給開發者）

| 路徑 | 內容 |
|------|------|
| `cmd/wikibuild` | 子命令 `mcp` → `runMCP()` |
| `internal/mcp/tools.go` | 業務邏輯（inmem 可測） |
| `internal/mcp/server.go` | mark3labs/mcp-go 註冊 tools + stdio |
| `internal/config` | `WIKIBUILD_MCP_TOKEN` |

單元測試：`go test ./internal/mcp/ -count=1`。
