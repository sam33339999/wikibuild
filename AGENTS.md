# AGENTS.md

Guidance for OpenCode sessions working in this repo. Compact, high-signal only.

## Repo status — read this first

This repo is **early scaffolding**. `README.md` is the design spec / roadmap (tech stack, data model, routes, MVP scope, milestones M0–M7) — **most of it is not implemented yet**. Do not assume code exists just because the README describes it.

Implemented today:
- `internal/model/` — domain types (`Article`, `User`), DB-agnostic
- `internal/store/store.go` — `Repository` interface + typed errors (`ErrNotFound`, `ErrDuplicateSlug`, `ErrEmptySlug`)
- `internal/store/inmem/` — in-memory `Repository` for unit tests (has passing tests)
- `internal/store/postgres/testhelper_test.go` — testcontainers helper only; **no real pg `Repository` yet**
- `internal/clock/` — `Clock` interface (`Real` / `Fake`) for time injection
- `Makefile`, `opencode.json`, `README.md`

NOT yet present (despite README): `cmd/`, `sqlc.yaml`, `db/`, `views/*.templ`, handlers, Fiber server, real postgres impl, migrations, static assets.

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

- `make generate` — target exists but fails: no `sqlc.yaml`, no `.templ` files, and sqlc/templ CLIs aren't wired up.
- `make run`, `make migrate-up`, `make migrate-down` — **no such Makefile targets**. README lists them as the target workflow for later milestones.

## Testing quirks

- Integration tests carry `//go:build integration`; plain `go test ./...` skips them — unit-only runs look low-coverage by design.
- `go mod tidy` keeps the testcontainers deps even though they're imported only under the build tag. This is correct; don't strip them.
- Integration tests need **Docker**; no manual Postgres required (testcontainers provisions it).

## Conventions

- Module path: `github.com/sam33339999/wikibuild` (all imports use it).
- Future generated code (`internal/store/sqlc/`, `views/*_templ.go`) must never be hand-edited.
- `opencode.json` sets `permission: "allow"` project-wide.
- README = design source of truth; this file = how to actually work here right now.
