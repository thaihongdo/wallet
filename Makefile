SHELL := /bin/bash
DATABASE_URL ?= postgres://wallet:wallet@localhost:5432/wallet?sslmode=disable
MIGRATE_IMAGE := migrate/migrate:v4.17.1

.PHONY: up down migrate-up migrate-down run test test-integration lint arch-check docker-build

up:
	docker compose up -d

down:
	docker compose down -v

migrate-up:
	docker run --rm --network host -v "$(CURDIR)/migrations:/migrations" $(MIGRATE_IMAGE) \
		-path=/migrations -database "$(DATABASE_URL)" up

migrate-down:
	docker run --rm --network host -v "$(CURDIR)/migrations:/migrations" $(MIGRATE_IMAGE) \
		-path=/migrations -database "$(DATABASE_URL)" down 1

run:
	set -a; [ -f .env ] && source .env; set +a; go run ./cmd/api

test:
	go test -race -count=1 ./internal/...

test-integration:
	go test -race -count=1 -tags=integration ./test/...

lint:
	golangci-lint run

arch-check:
	@echo "checking internal/usecase and internal/domain do not depend on gin/pgx/go-redis..."
	@deps=$$(go list -deps ./internal/usecase ./internal/domain); \
	if echo "$$deps" | grep -E 'gin-gonic/gin|jackc/pgx|redis/go-redis'; then \
		echo "FAIL: forbidden dependency found in usecase/domain"; exit 1; \
	else \
		echo "OK: no forbidden dependencies"; \
	fi

docker-build:
	docker build -t go-wallet:local .
