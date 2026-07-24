.PHONY: fmt lint test test-integration frontend-test build build-go build-web up smoke-compose check agent-pack

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
	cd web && npm run typecheck && npm run test && npm run build

# build-go needs no Node toolchain, so a Go-only environment can use it. build
# keeps its full meaning for a local release gate.
build-go:
	go build ./cmd/api ./cmd/worker

build-web:
	cd web && npm run build

build: build-go build-web

up:
	docker compose up --build --wait

smoke-compose:
	docker compose up --build --wait
	sh scripts/compose-smoke.sh

agent-pack:
	python3 scripts/check-agent-pack.py

check: fmt lint test test-integration frontend-test build agent-pack
