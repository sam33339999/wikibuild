# WikiBuild

個人用的 **blog + wiki**：一支 Go binary、PostgreSQL、後台寫 Markdown 或上傳整包 HTML，前台 SSR 給人讀也給搜尋引擎讀。

適合「自己的工程手帳／作品集／知識庫」——不是多使用者 CMS，也不是靜態站產生器工作流。

---

## 這專案在幹嘛

- **寫**：後台一篇篇管——草稿／發布、標籤、置頂、排程、密碼文、預覽連結。
- **雙軌內容**：
  - **Markdown** — 後台編輯，server 渲染（TOC、程式碼高亮、`[[wikilink]]`）。
  - **HTML 上版** — zip／單檔靜態頁或簡報；可整頁原檔直送，或嵌在站台外框（iframe）裡。
- **誰看得到**：`public` / `protected`（密碼）/ `private`（外人當 404）。
- **被人找到**：RSS／Atom／JSON Feed、sitemap、robots、可編 SEO meta／OG／JSON-LD（可 AI 輔助填）。
- **長相**：無 npm 建置的主題（深淺色）、品牌設定；閱讀頁可關目錄、懸浮大綱。
- **寫作輔助**：後台站內搜尋插 `[[wikilink]]`；可選 LLM 產生摘要／meta。
- **代理整合**：[MCP stdio](docs/integrations-mcp.md) 讀寫文章（`wikibuild mcp`）。

**v1.0 + v1.1 已可用。** 規格紀錄：[`docs/specs/v1.1-ai-seo-mcp.md`](docs/specs/v1.1-ai-seo-mcp.md)。

---

## 主要差異（為什麼不是 X）

| 對比 | WikiBuild 的取捨 |
|------|------------------|
| **vs WordPress / 多使用者 CMS** | 單 admin、schema 精簡、無外掛市集；部署面積極小。 |
| **vs Hugo / Jekyll 靜態站** | 動態後台 + DB，不必為一篇文跑 build／推 git 才能發；仍可上傳「整包靜態」當一篇。 |
| **vs Notion / 封閉 wiki** | 自架、自有資料與 URL；feed／sitemap／SSR，內容在自己的網域上。 |
| **vs 全站 SPA 後台** | **templ SSR + script tag** 前端，**零 npm**；`go build` 一個檔為主。 |
| **vs 純 Markdown git wiki** | 有可見性、密碼文、排程、HTML 簡報上版、媒體上傳，不必自己拼一堆工具。 |

一句話：**個人向、雙軌內容（MD + 完整 HTML）、三態隱私、單一 binary——偏「工程師自己的站」，不是企業 CMS。**

---

## Tech stack

| 層 | 選擇 | 備註 |
|----|------|------|
| 語言 | **Go 1.26** | |
| HTTP | **Fiber v3** | |
| DB | **PostgreSQL** + **pgx** | |
| 查詢 | **sqlc** | SQL → 型別安全 Go，不手寫 ORM 地獄 |
| Migration | **golang-migrate** | |
| 頁面 | **templ** | 型別安全 SSR |
| Markdown | **Goldmark** | GFM、TOC、chroma |
| 後台編輯 | **Vditor**（CDN） | IR / WYSIWYG / 源碼 |
| 前端 | CSS/JS + 可選 Alpine/HTMX | **無 SPA 框架、無 npm build** |
| 密碼／Session | bcrypt、HMAC cookie | |

**刻意不做的：** React/Vue 全家桶、Prisma 類重 ORM、多租戶、外掛系統。

實作慣例與指令：[`AGENTS.md`](AGENTS.md)。規格與路線圖：[`docs/`](docs/README.md)。

---

## 很快跑起來

```bash
cp .env.example .env   # admin、SESSION_SECRET、DATABASE_URL…
make db-up             # Postgres
make migrate-up
make run               # 需本機有 sqlc/templ/migrate 時再 make generate
```

細節（環境變數、路由、測試、部署）不堆在 README——見 **AGENTS.md** 與 **docs/**。

---

## 狀態

| | |
|--|--|
| **現在** | **v1.1 complete**（v1.0 之上）：SEO · AI SEO（MD+HTML）· 站內搜尋 / AI 相關建議 · 自動 OG 圖 · [MCP](docs/integrations-mcp.md) |
| **規格** | [v1.1](docs/specs/v1.1-ai-seo-mcp.md) · [CHANGELOG](docs/CHANGELOG.md) |
