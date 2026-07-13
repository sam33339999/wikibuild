# AGENTS.md

Guidance for OpenCode sessions working in this repo. Compact, high-signal only.

## Repo status — read this first

- **`README.md`** — what the project is, tech stack, differentiators (keep short).
- **`docs/specs/`** — planned work. Next: [`docs/specs/v1.1-ai-seo-mcp.md`](docs/specs/v1.1-ai-seo-mcp.md).
- **`AGENTS.md`** (this file) — how to run, architecture seams, commands.

**M0–M7 COMPLETE → v1.0 delivered.** Do not re-propose the locked stack or re-implement shipped milestones unless fixing bugs.

Implemented (high level):
- `internal/model/` — `Article` (`ShowTOC`, `PublishAt`, `PreviewToken`, `Pinned`, …), `User`, `Redirect`
- `internal/config/`, `clock/`, `auth/`, `gate/`, `render/`, `media/`, `feed/`, `scheduler/`, `seo/`, `sitebrand/`
- `internal/store/` — Repository + inmem + postgres(sqlc); settings, tags, redirects
- `internal/handler/` + `internal/server/` — full admin/public surface; `/static/*`
- `views/` — layout (theme, auto SEO meta/JSON-LD), admin, public (floating TOC)
- `db/migrations/` through `000005_show_toc`
- `static/` — site.css (Claude-adjacent reading UI), toc-sidebar.js, editor, theme
- `cmd/wikibuild`, `cmd/resetadmin`

**Not in admin UI yet:** per-article SEO title / meta description / OG image (auto-only). Spec: v1.1 S1.

## Toolchain (must be on PATH)

```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/a-h/templ/cmd/templ@latest
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Architecture seam (how to add features)

Handlers and logic depend **only on `store.Repository`**. Workflow:
1. Unit test against `inmem.New()` (fast, no DB).
2. Implement logic/handler.
3. Integration test (`//go:build integration`) against postgres when touching SQL.

- Inject `clock.Clock` for time; never `time.Now()` in tested logic.
- Assert with `errors.Is(err, store.ErrNotFound)`, etc.
- `model.Article` nullable times `*time.Time`; pg maps `pgtype` / nulls.
- **Clone form values** before persist (`strings.Clone` / `articleFromForm`).
- **Fiber route order:** static paths before `/:slug` / `/:id` at same depth.
- Nil `Tags` → `[]string{}` in pg layer.
- Never hand-edit `internal/store/sqlc/` or `*_templ.go`; run `make generate`.

## Locked tech stack — don't propose alternatives

Go 1.26 · Fiber v3 · PostgreSQL + pgx · **sqlc** · **templ** · golang-migrate · Vditor + script-tag front-end. **No SPA, no npm build.**

## Commands

```
make generate          # sqlc + templ
make run               # needs DATABASE_URL + admin env; schema via migrate-up first
make build
make migrate-up | migrate-down
make test              # unit only
make test-integration  # Docker / testcontainers
make vet | cover | fmt | tidy
make db-up | db-down | db-logs
```

## Dev database + .env

- `compose.yaml` — Postgres 16 only; app on host.
- `.env` from `.env.example` (gitignored). Compose + godotenv + Makefile `-include`.

## Testing quirks

- Integration tests: `//go:build integration`; plain `go test ./...` skips them.
- Keep testcontainers in `go.mod` even if only used under the build tag.

## Conventions

- Module: `github.com/sam33339999/wikibuild`
- New product work: implement against an existing **spec under `docs/specs/`**, or add a spec first for multi-slice features.
- `opencode.json` permission allow project-wide.
