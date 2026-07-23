.PHONY: fmt lint test test-integration frontend-test build up smoke-compose check agent-pack

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

build:
	go build ./cmd/api ./cmd/worker
	cd web && npm run build

up:
	docker compose up --build --wait

smoke-compose:
	docker compose up --build --wait
	sh scripts/compose-smoke.sh

agent-pack:
	python3 scripts/check-agent-pack.py

check: fmt lint test test-integration frontend-test build agent-pack
