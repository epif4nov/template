SHELL := /bin/bash

.PHONY: migrate migrate-down migrate-status run generate test build

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
	go tool oapi-codegen \
		-generate types,chi-server \
		-package api \
		-o internal/generated/api.gen.go \
		contracts/openapi/trip-service.openapi.yaml

test:
	@set -e; set -a; source .env; set +a; \
	CGO_ENABLED=1 go test -race -count=2 ./...

build:
	go build ./...