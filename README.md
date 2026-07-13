# WikiBuilder

個人用的 blog / wiki 系統：後台管理、Markdown 編輯、HTML 靜態檔直接上版，並支援每篇文章設定 `published` / `protected` / `private` 三種可見性。

## 特色

- **單一管理者後台**：建立 / 編輯 / 刪除 / 發布文章
- **雙軌內容**：
  - `markdown` — 後台編輯器撰寫，server 端渲染並套佈景
  - `html_upload` — 上傳完整 `.html`（可含資產資料夾）直接上版
- **HTML 上版呈現**：每篇 `raw_mode` 開關
  - 關：內容注入佈景
  - 開：整頁原檔直送（連佈景都不套）
- **可見性三態**：
  - `public` — 任何人可見
  - `protected` — 需密碼（全站預設密碼，每篇可覆寫）
  - `private` — 僅登入 admin 可見（未登入回 **404** 而非 403，避免曝光文章存在）
- **單一二進位檔部署**：`go build` 出一個執行檔 + 一個設定來源即可

## 技術棧

| 層 | 選擇 | 說明 |
|---|---|---|
| 語言 | Go 1.26 | |
| HTTP 框架 | [Fiber](https://gofiber.io) v3 | fasthttp 基底，輕量高效 |
| 資料庫 | PostgreSQL（`pgx` 驅動） | |
| DB 存取 | [sqlc](https://sqlc.dev) | 寫 SQL → 生成型別安全程式碼 |
| DB 抽象 | `Repository` interface | pg 實作為主，可換其他 DB |
| 視圖 (SSR) | [templ](https://templ.guide) | 型別安全的 Go 模板，所有頁面／表單結構 |
| 前端互動層 | [Alpine.js](https://alpinejs.dev) | 反應式互動：開關、modal、拖拉、自動儲存（~15KB，無建置） |
| AJAX 操作 | [HTMX](https://htmx.org) | 列表刷新、批次操作、即時搜尋、slug 檢查，免寫 fetch（~14KB，無建置） |
| Markdown 編輯器 | [Vditor](https://b3log.org/vditor/) | 後台即時渲染（IR）/ WYSIWYG / 源碼，穩定 CDN（UMD） |
| 內容渲染（前台） | highlight.js / Mermaid / KaTeX | 程式碼、圖表、數學，**按需 lazy-load** |
| Migrations | [golang-migrate](https://github.com/golang-migrate/migrate) | SQL 版控 |
| 密碼雜湊 | bcrypt | |
| Session | HMAC 簽章 cookie | protected 文章授權用 |

> **前端策略**：不導入 SPA 框架。templ 負責 SSR（前台 SEO、後台結構），Alpine + HTMX 以 script tag 注入互動，Vditor 作為後台 Markdown 編輯器（IR 即時渲染）掛入 templ。零 npm 建置，`go build` 仍為單一成品。僅當單一功能元件生態吃重（如知識圖譜）時，才為該頁引入專用庫，非全站框架。

## 目錄結構

```
wikibuild/
├── cmd/wikibuild/main.go          # 程式進入點
├── sqlc.yaml                      # sqlc 設定
├── db/
│   ├── migrations/                # golang-migrate 的 .up/.down.sql
│   └── queries/                   # sqlc 的 .sql 查詢定義
├── internal/
│   ├── config/                    # 環境變數 / 設定載入
│   ├── model/                     # domain 型別（Article, User ...）
│   ├── store/
│   │   ├── store.go               # Repository interface（抽象邊界）
│   │   ├── postgres/              # Postgres 實作（包裝 sqlc.Queries）
│   │   └── sqlc/                  # sqlc 生成的程式碼（勿手改）
│   ├── auth/                      # bcrypt / session + Fiber middleware
│   ├── gate/                      # public / protected / private 可見性中介層
│   ├── render/                    # markdown → html、html 注入佈景
│   ├── handler/                   # Fiber handlers（admin / public 兩組）
│   └── server/                    # Fiber app 實例 + 路由組裝
├── views/                         # *.templ 原始檔（編譯成 *_templ.go）
│   ├── layout/                    # 共用版型
│   ├── admin/                     # 後台頁面
│   └── public/                    # 公開頁面
├── static/                        # 佈景 css/js + Alpine/HTMX/編輯器（本地化或 CDN tag）
│   ├── vendor/                    # 第三方 JS（alpine, htmx, milkdown, highlight.js…）
│   └── css/                       # 佈景樣式
├── content/uploads/               # 上傳的 html 與附帶資產
├── Makefile                       # generate / migrate / run 快捷指令
└── go.mod
```

## 資料模型

```
Article
  id, slug (unique), title
  type        : markdown | html_upload
  status      : draft | published
  visibility  : public | protected | private
  password    : protected 文章密碼（bcrypt），可空（用全站預設）
  raw_mode    : bool，html_upload 是否原檔直送
  body        : markdown 內文；html_upload 則記錄檔案路徑
  tags
  created_at, updated_at, published_at

User
  id, username, password_hash (bcrypt)
```

## 可見性流程

| 可見性 | 未登入 | 已登入 admin | 授權方式 |
|---|---|---|---|
| public | 渲染 | 渲染 | 無 |
| protected | 顯示密碼頁 | 渲染 | 輸對密碼 → 設 HMAC 簽章 cookie（帶文章 id + 過期） |
| private | **404** | 渲染 | 需 admin session |

Protected 密碼比對優先序：**文章密碼 > 全站預設密碼**。簽章 cookie 綁定單一文章 id，防跨篇洩漏。

## 路由規劃

### 公開

```
GET  /                文章列表（分頁）
GET  /search?q=       全文搜尋（published + public）
GET  /archive         按年月封存索引
GET  /archive/:year   某年文章
GET  /archive/:year/:month  某月文章
GET  /tag/:tag        標籤過濾
GET  /media/:name     上傳的圖片（貼圖／拖拉）
GET  /feed            RSS 2.0
GET  /feed/atom       Atom 1.0
GET  /feed.json       JSON Feed 1.1
GET  /sitemap.xml     sitemap
GET  /robots.txt      robots
GET  /preview/:token  草稿預覽分享連結（unlisted）
GET  /:slug           文章內容（未知 slug 查 301 導向）
GET  /:slug/unlock    protected 密碼驗證頁 / 處理
```

### 後台（需 admin session）

```
GET  /admin/login     登入頁
POST /admin/login     登入處理
POST /admin/logout    登出

GET  /admin?q=        文章列表（可搜尋標題／內文）
GET  /admin/new       新增 Markdown 文章
POST /admin/new       建立文章
GET  /admin/upload    上傳 HTML 上版頁
POST /admin/upload    處理上傳
POST /admin/media     圖片上傳（回 JSON {url,name}，編輯器貼圖／拖拉用）
GET  /admin/tags      標籤管理（列表／計數）
POST /admin/tags/rename  標籤重新命名
POST /admin/tags/merge   標籤合併
GET  /admin/redirects 導向管理
POST /admin/redirects 新增導向
POST /admin/redirects/delete 刪除導向
GET  /admin/:id/edit  編輯（含 status / visibility / password / raw_mode / pinned / 排程 / 預覽連結）
POST /admin/:id       更新（slug 變更自動建 301）
POST /admin/:id/delete 刪除
GET  /admin/settings  全站設定（protected 密碼、留言 giscus/utterances）
```

## 開發

### 前置需求

- Go 1.26+
- PostgreSQL（開發可用 `docker compose` 一鍵起，見下方）
- `sqlc` CLI、`templ` CLI、`migrate` CLI（或透過 Makefile / go generate 呼叫）

### 建置步驟（程式碼生成）

```bash
make generate     # 同時跑 sqlc generate + templ generate
# 或分別執行：
sqlc generate
templ generate
```

### 資料庫迁移

```bash
make migrate-up     # 套用 migrations
make migrate-down   # 回退一步
```

### 啟動

```bash
cp .env.example .env   # 編輯設定（DB 連線、admin 帳密、session secret）
make db-up             # docker compose 起 PostgreSQL（開發用）
make migrate-up        # 套用 schema
make run               # go run ./cmd/wikibuild（自動載入 .env）
```

## 設定（環境變數）

| 變數 | 說明 | 預設 |
|---|---|---|
| `DATABASE_URL` | PostgreSQL 連線字串 | — |
| `WIKIBUILD_HOST` | 監聽位址（`127.0.0.1` 僅本機；`0.0.0.0` 對外） | `127.0.0.1` |
| `WIKIBUILD_PORT` | 監聽 port | `8080` |
| `WIKIBUILD_ADMIN_USER` | 管理者帳號 | — |
| `WIKIBUILD_ADMIN_PASS` | 管理者初始密碼（首次啟動建帳號用） | — |
| `WIKIBUILD_SESSION_SECRET` | session HMAC 簽章金鑰 | — |
| `WIKIBUILD_CONTENT_DIR` | 上傳內容目錄 | `./content/uploads` |
| `WIKIBUILD_DEFAULT_PROTECTED_PASS` | protected 文章全站預設密碼 | — |
| `WIKIBUILD_BASE_URL` | 站台絕對網址（feeds/sitemap/canonical，無尾隨 `/`） | — |
| `WIKIBUILD_SITE_TITLE` | 站台／feed 標題 | `WikiBuild` |

### `.env` 機制

`.env` 為開發單一設定來源：docker compose 讀 `POSTGRES_*`，app 透過 `godotenv` 自動載入（真實環境變數仍優先），Makefile 透過 `-include .env` 讓 `make migrate-up` 取得 `DATABASE_URL`。`.env.example` 為 committed 樣板，`.env` 本身 gitignore。

## 部署

`go build -o wikibuild ./cmd/wikibuild` 產出單一執行檔，搭配一個 PostgreSQL 與一份設定（環境變數或 `.env`）即可運作。靜態資產與上傳內容分別放在 `static/` 與 `content/uploads/`。

## 測試策略

採分層測試金字塔搭配可注入設計：大部分邏輯以純單元測試驅動（TDD），DB 正確性另以整合測試驗證。

### 測試金字塔

| 層 | 對象 | 依賴 | 速度 |
|---|---|---|---|
| L1 純函式 | markdown→html、TOC、wikilink 解析、閱讀時間、bcrypt | 無 | 極快 |
| L2 邏輯/中介 | visibility gate、CSRF、限流、session | 介面注入 | 極快 |
| L3 Handler | Fiber handler + inmem store | inmem | 快 |
| L4 Repository (pg) | sqlc 包裝、SQL、約束 | 真 Postgres（testcontainers） | 中 |
| L5 HTTP/templ | 路由、middleware、渲染輸出 | inmem 或 pg | 快/中 |
| L6 E2E | 瀏覽器：編輯器、上傳 | Playwright / agent-browser | 慢 |

### 可測性設計（已內建於骨架）

- `store.Repository` 介面：handler／邏輯層只依賴介面，測試注入 `inmem` 實作，**不碰 DB**。
- `clock.Clock` 介面：排程發布與時間戳可注入假時鐘。
- 內容目錄可注入：上傳測試用 `t.TempDir()`。
- 型別錯誤（`store.ErrNotFound` / `store.ErrDuplicateSlug`）：以 `errors.Is` 斷言。

### DB 整合測試（testcontainers）

L4 postgres 測試以 [testcontainers-go](https://github.com/testcontainers/testcontainers-go) 每 run 起一個 ephemeral Postgres 容器，套用 migrations 後驗證 SQL／約束。整合測試以 build tag `integration` 隔離：

```bash
make test              # 預設只跑單元測試（快、不需 docker）
make test-integration  # 跑 L4/L5 整合測試（需 docker）
make cover             # 覆蓋率報告
```

- 整合測試檔標註 `//go:build integration`。
- 測試輔助：`internal/store/postgres/testhelper_test.go` 提供 `StartPostgres(t)`。

### 每功能 TDD 循環

1. 先寫 L1/L2 測試（純邏輯）→ Red
2. 實作最小邏輯 → Green
3. 重構
4. 補 L3 handler 測試（inmem）驗證接線
5. 補 L4 postgres 測試驗證 SQL／約束
6.（選）L6 E2E 驗收

### 注意

- sqlc 從 SQL 生成，無法對 SQL 做 strict TDD；折衷為「對 Repository 介面先寫測試（紅）→ 寫 pg wrapper → 整合測試（綠）」。
- 時間、隨機（token/salt）、檔案路徑一律注入，測試才可重現。
- 不測 sqlc／templ 生成碼本身，只測包裝與行為。

## MVP 範圍 (v1.0)

經篩選，v1.0 上線版鎖定下列範圍。核心需求與技術棧為必做基底，候選功能依下表取捨。

### IN（納入 v1.0）

| 分類 | 功能 |
|---|---|
| 核心（必做） | 文章 CRUD、Markdown 發文、HTML 靜態上版（含 `raw_mode`）、`published`/`protected`/`private` 三態可見性 |
| 安全 | CSRF 防護、登入限流 |
| 閱讀體驗 | 自動目錄（TOC）、深淺色主題、預估閱讀時間、按日期封存 |
| 內容發現 | 全文搜尋（管理 + 讀者）、標籤管理（重命名／合併）、置頂／精選 |
| 發布控制 | 排程發布、草稿預覽分享連結（unlisted）、導向管理（slug 變更自動 301） |
| 分發 | RSS / Atom / JSON Feed、sitemap + robots + SEO meta（OG/Twitter/JSON-LD）、留言（giscus/utterances） |
| 創作輔助 | 圖片上傳簡易版（貼圖／拖拉）、雙向連結 `[[wikilinks]]` |

### OUT（延後到 v1.1+）

版本歷史／修訂還原、備份／還原、系列文、相關文章、讀者端搜尋 UI、隱私友善分析、圖片最佳化／縮圖，以及 P2 探索項。

## 開發里程碑

> 里程碑依上述 MVP 範圍重排，標註涵蓋的分類。

1. **M0 基礎骨架**｜核心+安全 ✅ 已完成
   config、PostgreSQL schema、migrations、Repository interface + pg 實作、Fiber app、templ 版型、admin 登入（bcrypt + session）、**CSRF 中介層、登入限流**、docker compose 開發 DB、`.env` 機制
2. **M1 文章核心（Markdown）**｜核心 ✅ 已完成
   後台列表／新增／編輯／刪除、Goldmark 渲染、**自動目錄（TOC）**、程式碼高亮、公開頁渲染、列表分頁、slug 唯一、status 草稿／發布
3. **M2 可見性三態**｜核心 ✅ 已完成
   public / protected（密碼頁＋HMAC 簽章 cookie，全站預設＋每篇覆寫）/ private（未登入回 404）、settings 頁
4. **M3 HTML 靜態上版**｜核心 ✅ 已完成
   上傳 `.html`（含資產）→ `content/uploads/<slug>/`、`raw_mode` 切換（套佈景／原檔直送）
5. **M4 內容豐富化**｜創作+閱讀 ✅ 已完成
   **圖片上傳簡易版（貼圖／拖拉）**、**雙向連結 `[[wikilinks]]` → 連回連結**、**預估閱讀時間**、**標籤管理（重命名／合併）**、**置頂／精選**
6. **M5 發現與導航**｜發現 ✅ 已完成
   **全文搜尋（管理 + 讀者）**、**按日期封存**、標籤頁
7. **M6 發布與分發**｜發布+分發 ✅ 已完成
   **排程發布（背景排程器）**、**草稿預覽分享連結**、**導向管理**、**RSS/Atom/JSON Feed**、**sitemap + robots + SEO meta**、**留言（giscus/utterances）**
8. **M7 前台體驗收尾**｜閱讀 ✅ 已完成
   **深淺色主題切換**、佈景打磨、靜態資產、端到端測試 → v1.0 上線

---

## 未來規劃 / 候選功能

下列功能來自三輪腦力激盪，分別從「內容創作與管理」「閱讀體驗與讀者」「營運／安全／平台／整合」三個角度切入。優先級依**個人 blog/wiki** 的實用度與核心價值排序（非絕對，可隨使用情境調整）。

### 第一輪｜內容創作與管理（作者視角）

- 草稿自動儲存
- 版本歷史／修訂還原（wiki 本質，重要）
- 圖片上傳／媒體庫
- 剪貼簿貼圖、拖拉上傳
- Markdown 即時預覽（split pane）
- 自動目錄（TOC）
- 程式碼語法高亮
- 數學式渲染（KaTeX）
- Mermaid／圖表渲染
- 排程發布（`publish_at`）
- 置頂／精選文章
- 系列文（多篇串連）
- 相關文章推薦
- 全文搜尋（管理端 + 讀者端）
- 標籤管理（重命名／合併／描述）
- 分類與標籤階層
- 草稿預覽分享連結（unlisted）
- 匯入／匯出（Jekyll/Hugo markdown、HTML 批次匯入、備份）
- 導向管理（slug 變更自動 301）
- 雙向連結 `[[wikilinks]]`（wiki 核心，高價值）

### 第二輪｜閱讀體驗與讀者（讀者視角）

- 深淺色主題切換（含 auto）
- 閱讀進度條
- 預估閱讀時間
- 固定式目錄側欄
- 回到頂端
- 讀者端搜尋 UI
- 按日期封存（年／月）
- 標籤雲
- 分頁或無限捲動
- 留言系統（giscus/utterances 或自架 isso）
- 按讚／互動
- 電子報／Email 訂閱
- RSS / Atom / JSON Feed
- sitemap.xml + robots.txt
- SEO：meta、Open Graph、Twitter Card、JSON-LD
- OG 圖片自動產生
- 社群分享按鈕
- 列印樣式 / PDF 匯出
- 隱私友善分析（Plausible/Umami 自架，或內建計數器）
- 多語系內容
- 自訂 404 頁

### 第三輪｜營運、安全、平台、整合（維運視角）

- Webhook／API 程式化發布（CI 推送）
- CLI 推文工具（`wikibuild push article.md`）
- Git 內容同步（內容存 git repo，自動 pull）
- 備份與還原（DB dump + content 目錄）
- 健康檢查 endpoint
- 登入限流（防暴力破解）
- 2FA / TOTP
- CSRF 防護（admin 表單）
- 安全標頭（CSP、HSTS）
- 自動 TLS（Let's Encrypt autocert）
- 結構化日誌／可觀測性指標
- HTTP 快取、靜態資產指紋
- 多站／多 wiki 共用一實例
- 主題系統／主題切換／自訂主題上傳
- 外掛／擴充系統
- Webmentions / IndieWeb / Micropub（行動裝置發文）
- ActivityPub / Fediverse（部落格成為聯邦節點）
- 圖片最佳化／縮圖／WebP 轉換
- 上傳大小限制與檔案類型驗證
- Docker / docker-compose
- systemd 服務檔
- 稽核日誌（admin 操作紀錄）
- API token（整合其他工具）
- WebSub（即時 feed 更新通知）
- 留言垃圾防護（honeypot、hCaptcha）

### 彙整：候選功能清單與優先級

#### P0 高｜核心，建議近期實作

| 功能 | 理由 |
|---|---|
| 全文搜尋（管理 + 讀者） | 內容一多就必需 |
| 版本歷史／修訂還原 | wiki 本質，個人資料安全 |
| 圖片上傳／媒體庫（含貼圖、拖拉） | 撰文基本需求 |
| Markdown 即時預覽 | 降低作者摩擦 |
| 自動目錄 + 程式碼高亮 | 閱讀基本品質 |
| 雙向連結 `[[wikilinks]]` | wiki 核心價值，差異化 |
| RSS / Atom / JSON Feed | blog 標配 |
| sitemap + robots + SEO meta | 內容被看見的基礎 |
| 排程發布 | 常見需求 |
| 深淺色主題 | 讀者期待 |
| 備份／還原 | 個人資料不能丟 |
| CSRF 防護 + 登入限流 | 安全底線 |

#### P1 中｜有價值，列入中期規劃

| 功能 | 理由 |
|---|---|
| 預估閱讀時間 | 低成本高體感 |
| 按日期封存 | 內容導航 |
| 標籤管理（重命名／合併） | 維護效率 |
| 置頂／精選、系列文、相關文章 | 編輯彈性 |
| 草稿預覽分享連結 | 協作／預覽便利 |
| 導向管理 | slug 變更不失流量 |
| 讀者端搜尋 UI | 落實全文搜尋價值 |
| 隱私友善分析 | 了解讀者又不侵犯隱私 |
| Webhook/API + CLI 推文 | power user 自動化 |
| Git 內容同步 | 版控 + 多機撰寫 |
| 圖片最佳化／縮圖 | 效能與流量 |
| 留言（giscus/utterances） | 讀者互動，低成本整合 |
| 自動 TLS（autocert） | 部署省事 |
| Docker / compose | 部署標準化 |
| 列印/PDF 樣式 | 內容再利用 |

#### P2 低 / 未來探索

電子報訂閱、OG 圖自動產生、多語系、自訂 404、2FA/TOTP、多站、主題系統、外掛系統、Webmentions/IndieWeb/Micropub、ActivityPub/Fediverse、API token、WebSub、知識圖譜視覺化、AI 輔助（標題／摘要／翻譯／標籤建議）、留言垃圾防護、稽核日誌。

> 以上為候選清單，非承諾範圍。實作時可依實際使用回饋調整優先級。
