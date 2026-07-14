# Changelog

## v1.1 — 2026-07-14

SEO control, AI assist, editor search, and MCP. Spec: [`specs/v1.1-ai-seo-mcp.md`](./specs/v1.1-ai-seo-mcp.md).

### S1 — Editable SEO & social fields
- Article columns: `seo_title`, `summary`, `meta_description`, `cover_image_url`, `og_image_url`
- Admin「SEO / 分享」form; public meta / OG / JSON-LD fallbacks; feed summary prefers author summary
- Rune-safe body clip (`internal/seo`)

### S2 — AI 產生 SEO
- OpenAI-compatible `chat/completions` (`internal/llm`)
- Admin button pre-fills summary + meta (save required); outline UI-only
- Env: `WIKIBUILD_LLM_BASE_URL`, `WIKIBUILD_LLM_API_KEY`, `WIKIBUILD_LLM_MODEL`

### S3a — Editor site search
- `GET /admin/api/articles/search`
- Side panel inserts `[[slug]]` wikilinks

### S4 — MCP
- `wikibuild mcp` stdio server; `WIKIBUILD_MCP_TOKEN` required
- Tools: list/get/create/update, set status/visibility (create defaults draft+private)
- Docs: [`integrations-mcp.md`](./integrations-mcp.md)

### Follow-ups (same track, TDD)
- **S3b** — `POST /admin/ai/related` + editor「AI 相關建議」
- **HTML AI SEO** — strip upload HTML to plain text for GenerateSEO
- **Auto OG** — `internal/ogimage` 1200×630 PNG → `/media/…` via `POST /admin/:id/ai/og`

### LLM Streaming Playground
- Admin **LLM Streaming Playground** (`/admin/playground`)
- Multi-turn history + `POST /admin/ai/chat/stream` SSE
- OpenAI-compatible `stream: true`; live Markdown (marked + DOMPurify)
- Docs: [`llm-playground.md`](./llm-playground.md)

### Ops / deploy notes
- Login CSRF failures redirect to `?err=csrf|cred|locked` with readable messages
- CSRF cookies tuned for **TLS at Nginx Proxy Manager** (`CookieSecure=false`, TrustProxy)
- Makefile falls back to `go run` when `migrate` CLI missing
- Docs: [`deploy-nginx.md`](./deploy-nginx.md)（含 NPM 調整與上線 CSRF 事故紀錄）

## v1.0

M0–M7: CRUD, visibility, MD/HTML upload, search/tags/archive, feeds/sitemap, theme, TOC, schedule/preview, etc.
