# AGENTS.md

Guidance for OpenCode sessions working in this repo. Compact, high-signal only.

## Repo status — read this first

- **`README.md`** — what the project is, tech stack, differentiators (keep short).
- **`docs/specs/`** — v1.1 fully shipped: [`docs/specs/v1.1-ai-seo-mcp.md`](docs/specs/v1.1-ai-seo-mcp.md).
- **`AGENTS.md`** (this file) — how to run, architecture seams, commands.

**v1.0 (M0–M7) + v1.1 complete** (S1–S4, S3b, HTML AI SEO, auto OG). Do not re-propose the locked stack or re-implement shipped milestones unless fixing bugs.

Implemented (high level):
- `internal/model/` — `Article` (SEO fields, `ShowTOC`, `PublishAt`, `PreviewToken`, `Pinned`, …), `User`, `Redirect`
- `internal/config/`, `clock/`, `auth/`, `gate/`, `render/`, `media/`, `feed/`, `scheduler/`, `seo/`, `ogimage/`, `sitebrand/`, `llm/`, `mcp/`
- `internal/store/` — Repository + inmem + postgres(sqlc); settings, tags, redirects
- `internal/handler/` + `internal/server/` — admin/public; AI SEO/related/OG; editor search; `/static/*`
- `views/` — layout (SEO meta/JSON-LD), admin (SEO, AI, search, related), public (floating TOC)
- `db/migrations/` through `000006_article_seo`
- `static/` — site.css, chroma.css, toc-sidebar.js, editor, editor-search.js, ai-seo.js, theme
- `cmd/wikibuild` (HTTP + `mcp` subcommand), `cmd/resetadmin`

## Toolchain (must be on PATH)

```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/a-h/templ/cmd/templ@latest
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Architecture seam (how to add features)

Handlers and logic depend **only on `store.Repository`**.

### TDD is required (red → green → refactor)

Do **not** implement first and bolt tests on later. For each behaviour slice:

1. **Red** — write a failing unit test that names the desired behaviour (prefer `inmem.New()` / fake collaborators; no live LLM / no DB).
2. **Green** — minimal code to pass that test.
3. **Refactor** — clean up only while tests stay green.
4. Repeat; keep the suite green between slices.
5. When touching SQL: add `//go:build integration` tests against postgres **after** the unit path is green (or in parallel only if the contract is already fixed by unit tests).

External I/O (LLM HTTP, MCP stdio): define an **interface**, mock in unit tests, one real HTTP/stdio adapter tested with a fake transport or golden fixtures.

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
