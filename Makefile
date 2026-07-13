.PHONY: generate migrate-up migrate-down migrate-force run build db-up db-down db-logs test test-unit test-integration cover tidy fmt vet

# Codegen tools (sqlc, templ, migrate) are expected on PATH; see README for
# install instructions. They can also be invoked via `go run`.

# Pull in .env if present so DB_URL / DATABASE_URL etc. are available to
# targets below. Missing file is fine (leading dash). Existing env vars in the
# shell still win for the app via godotenv.
-include .env

generate:
	sqlc generate
	templ generate

# DATABASE_URL is read from .env (via -include) or the shell. The default
# matches compose.yaml + .env.example.
DATABASE_URL ?= postgres://wikibuild:wikibuild@localhost:5432/wikibuild?sslmode=disable

migrate-up:
	migrate -path db/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path db/migrations -database "$(DATABASE_URL)" down 1

# Force a migration version (recovery helper). Usage: make migrate-force V=1
migrate-force:
	migrate -path db/migrations -database "$(DATABASE_URL)" force $(V)

run:
	go run ./cmd/wikibuild

build:
	go build -o wikibuild ./cmd/wikibuild

# ---- Docker compose (dev database) ----
db-up:
	docker compose up -d

db-down:
	docker compose down

db-logs:
	docker compose logs -f postgres

test: test-unit

test-unit:
	go test ./...

test-integration:
	go test -tags=integration ./...

cover:
	go test -race -coverprofile=cover.out ./...
	go tool cover -func=cover.out

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy
