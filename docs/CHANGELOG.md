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

### Not in v1.1 (optional later)
- S3b LLM related-article suggestions
- AI SEO for html_upload
- Auto-generated OG images

## v1.0

M0–M7: CRUD, visibility, MD/HTML upload, search/tags/archive, feeds/sitemap, theme, TOC, schedule/preview, etc.
