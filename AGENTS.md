# AGENTS.md

Guidance for OpenCode sessions working in this repo. Compact, high-signal only.

## Repo status — read this first

`README.md` is the design spec / roadmap (tech stack, data model, routes, MVP scope, milestones M0–M7). `AGENTS.md` = how to actually work here.

**M0 (基礎骨架 + 安全) is COMPLETE.** The app builds, runs against real Postgres, and supports admin login with bcrypt + HMAC session cookie, CSRF protection, and login rate-limiting.

Implemented:
- `internal/model/` — domain types (`Article`, `User`), DB-agnostic
- `internal/config/` — env loading (pure, `Load(lookup)`), 100% covered
- `internal/clock/` — `Clock` interface (`Real` / `Fake`) for time injection
- `internal/store/store.go` — `Repository` interface (Articles + Users) + typed errors (`ErrNotFound`, `ErrDuplicateSlug`, `ErrDuplicateUsername`, `ErrEmptySlug`, `ErrEmptyUsername`)
- `internal/store/inmem/` — in-memory `Repository` for unit tests
- `internal/store/sqlc/` — **sqlc-generated** (do not edit); regenerate with `make generate`
- `internal/store/postgres/` — real `Repository` impl wrapping sqlc; integration-tested (17 L4 tests, testcontainers)
- `internal/auth/` — `PasswordHasher` (bcrypt), HMAC `Signer` (session tokens), `LoginLimiter` (brute-force protection)
- `internal/handler/` — Fiber handlers; `AdminAuth` (login/logout/dashboard + `RequireAuth` middleware), unit-tested against inmem + fake hasher
- `internal/server/` — Fiber app assembly: recover + CSRF middleware (double-submit, header + `_csrf` form field), routes
- `views/admin/login.templ` — templ login page (generated → `login_templ.go`)
- `db/` — `schema.sql` (canonical, for sqlc), `migrations/` (golang-migrate), `queries/` (sqlc), `embed.go` (embeds migrations for integration tests)
- `cmd/wikibuild/main.go` — entry point: config → pgxpool → pg repo → ensureAdmin → server → graceful shutdown
- `Makefile` — `generate`, `migrate-up`/`migrate-down`/`migrate-force`, `run`, `build`, test targets all work

NOT yet present (M1+): admin article CRUD UI, Goldmark markdown rendering, public pages, visibility gate (`internal/gate/`), HTML upload, search/archive, RSS/sitemap, static assets/themes.

## Toolchain (must be on PATH)

`sqlc`, `templ`, `migrate` CLIs are required for `make generate` / `make migrate-*`. Install:
```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/a-h/templ/cmd/templ@latest
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Architecture seam (how to add features)

Handlers and logic depend **only on `store.Repository`**. Workflow:
1. Write a unit test against `inmem.New()` (fast, no DB).
2. Implement logic/handler.
3. Add an integration test (build tag `integration`) against the postgres impl once it exists.

- Inject `clock.Clock` for anything time-related (scheduled publish, timestamps). Never call `time.Now()` directly in logic under test.
- Assert errors with `errors.Is(err, store.ErrNotFound)`, etc.
- `model.Article` uses `*time.Time` for nullable timestamps; the pg layer must translate to/from `pgtype` / `sql.NullTime`.

## Locked tech stack — don't propose alternatives

Go 1.26.3 · Fiber v3 · PostgreSQL via `pgx` · **sqlc** (codegen) · **templ** (codegen) · golang-migrate · Alpine.js + HTMX + Milkdown (frontend). **No SPA framework, no npm build** — frontend is script-tag only. Rationale lives in README.

## Commands (verified)

```
make generate          # sqlc generate + templ generate (CLIs on PATH)
make run               # go run ./cmd/wikibuild (needs DATABASE_URL + admin env)
make build             # go build -o wikibuild ./cmd/wikibuild
make migrate-up        # applies db/migrations to DB_URL (default local pg)
make migrate-down      # rolls back one migration
make test              # unit tests ONLY (integration excluded by build tag)
make test-integration  # needs Docker (testcontainers → postgres:16-alpine)
make vet               # go vet ./...
make cover             # coverage
make fmt               # gofmt -w
make tidy              # go mod tidy
```

Single test / package:
```
go test ./internal/store/inmem/... -run TestCreateArticle_DuplicateSlug -v
```

There is **no separate lint or typecheck step**. Verify code with: `go build ./... && go vet ./...`.

## Commands that DON'T work yet (despite README)

- `make run` runs but the running server needs the schema applied first (`make migrate-up`) and the required env vars (`DATABASE_URL`, `WIKIBUILD_ADMIN_USER`, `WIKIBUILD_ADMIN_PASS`, `WIKIBUILD_SESSION_SECRET`).

## Dev database + .env

- `compose.yaml` runs Postgres 16 only (the app runs on the host). `make db-up` / `db-down` / `db-logs` wrap `docker compose`.
- `.env` is the single config source for dev: compose reads `POSTGRES_*` from it, the app loads it via `godotenv` (real env vars still override), and the Makefile `-include`s it so `make migrate-up` picks up `DATABASE_URL`. `.env.example` is the committed template (`cp .env.example .env`); `.env` itself is gitignored.

## Testing quirks

- Integration tests carry `//go:build integration`; plain `go test ./...` skips them — unit-only runs look low-coverage by design.
- `go mod tidy` keeps the testcontainers deps even though they're imported only under the build tag. This is correct; don't strip them.
- Integration tests need **Docker**; no manual Postgres required (testcontainers provisions it).

## Conventions

- Module path: `github.com/sam33339999/wikibuild` (all imports use it).
- Future generated code (`internal/store/sqlc/`, `views/*_templ.go`) must never be hand-edited.
- `opencode.json` sets `permission: "allow"` project-wide.
- README = design source of truth; this file = how to actually work here right now.
