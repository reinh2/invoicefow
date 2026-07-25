.PHONY: fmt lint test test-integration frontend-test test-e2e build build-go build-web up smoke-compose check agent-pack \
	dev dev-db dev-api dev-worker dev-receiver dev-web

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

lint:
	go vet ./...

test:
	go test ./...

test-integration:
	@test -n "$$DATABASE_URL" || (echo "DATABASE_URL is required for PostgreSQL integration tests" >&2; exit 1)
	go test -tags=integration ./...

frontend-test:
	cd web && npm run format:check && npm run lint && npm run typecheck && npm run test && npm run build

# Browser end-to-end tests. Unlike every other target these need a *running*
# demo, so they are deliberately not part of `check`. Point them at an isolated
# instance, never the default persistent one:
#
#   COMPOSE_PROJECT_NAME=invoiceflow-e2e API_HOST_PORT=18082 \
#     POSTGRES_HOST_PORT=15434 RECEIVER_HOST_PORT=18092 \
#     docker compose up --build --wait
#   E2E_BASE_URL=http://127.0.0.1:18082 make test-e2e
#
# One-time browser install: npm --prefix web exec -- playwright install chromium
test-e2e:
	cd web && npm run test:e2e

# build-go needs no Node toolchain, so a Go-only environment can use it. build
# keeps its full meaning for a local release gate.
build-go:
	go build ./cmd/api ./cmd/worker

build-web:
	cd web && npm run build

build: build-go build-web

# --- Local development (host processes, Postgres in Compose) -----------------
# `up` runs the whole product in Docker. These targets instead run the Go
# binaries and the Vite dev server on the host against a Compose Postgres, which
# is faster to iterate on. The API defaults already point at 127.0.0.1:8080 and
# the Compose database, and Vite proxies /api there (see web/vite.config.ts).

# Start only PostgreSQL and wait until it is healthy.
dev-db:
	docker compose up --wait postgres

# Each of these runs one part in the foreground.
dev-api:
	go run ./cmd/api

dev-worker:
	go run ./cmd/worker

# The webhook receiver is only exercised when WEBHOOK_URL/WEBHOOK_SECRET are set
# for the api and worker; run it alongside them when testing the webhook export.
dev-receiver:
	WEBHOOK_SECRET=$${WEBHOOK_SECRET:-local-demo-webhook-secret} go run ./cmd/receiver

dev-web:
	cd web && npm run dev

# Run the database, API, worker, and web dev server together. Ctrl-C stops all
# of them (the trap signals the whole process group).
dev: dev-db
	@echo "Starting api (127.0.0.1:8080), worker, and web dev server. Press Ctrl-C to stop all."
	@trap 'kill 0' INT TERM EXIT; \
		go run ./cmd/api & \
		go run ./cmd/worker & \
		( cd web && npm run dev ) & \
		wait

up:
	docker compose up --build --wait

smoke-compose:
	docker compose up --build --wait
	sh scripts/compose-smoke.sh

# Local-only: validates the coding-agent harness in .internal/, which is not
# part of the published repository. Deliberately excluded from `check`.
agent-pack:
	python3 .internal/check-agent-pack.py

check: fmt lint test test-integration frontend-test build
