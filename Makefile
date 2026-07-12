.PHONY: generate test test-unit test-integration cover tidy fmt vet

generate:
	sqlc generate
	templ generate

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
