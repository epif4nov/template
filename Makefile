SHELL := /bin/bash

.PHONY: migrate migrate-down migrate-status run generate test test-integration lint build

migrate:
	@set -a; source .env; set +a; \
	go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@set -a; source .env; set +a; \
	go tool goose -dir migrations postgres "$$DATABASE_URL" down

migrate-status:
	@set -a; source .env; set +a; \
	go tool goose -dir migrations postgres "$$DATABASE_URL" status

run:
	@set -a; source .env; set +a; \
	go run ./cmd/trip-service

generate:
	go generate -x ./internal/usecase/trip/... ./internal/http/trip/...
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

test:
	CGO_ENABLED=1 go test -race -count=2 ./...
	go test -cover ./internal/domain/... ./internal/usecase/...

test-integration:
	@set -e; set -a; if [[ -f .env ]]; then source .env; fi; set +a; \
	CGO_ENABLED=1 go test -race -count=2 -tags=integration ./internal/repository/trip/...

lint:
	golangci-lint run ./...

build:
	go build ./...
