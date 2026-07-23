# Current Task

## State

**Stage 4 — immutable human review and rejection is complete.** It extends the uncommitted Stage 3 foundation and deliberately stops before approval, CSV/webhook export, payment, product media, and final landing-scroll work.

## Implemented through Stage 4

- Accepted ADRs pin the local toolchain, licenses, hostile-PDF behavior, data/process limits, strict provider contract/evidence semantics, and the `money-v1` exact normalization policy.
- The worker claims durable process jobs, reopens server-owned originals, validates PDFs with `pdfinfo`, extracts bounded text with `pdftotext`, falls back through the OCR port when text is absent, invokes the deterministic no-network fake extractor, and atomically persists an immutable snapshot before moving the document to `needs_review`.
- The default `TesseractOCR` adapter supports revalidated JPEG/PNG inputs. PDF raster OCR is intentionally deferred; a scanned PDF has a stable permanent failure rather than a fabricated result.
- Proposal JSON has a narrow, unknown-field-rejecting decoder. Evidence is persisted only when its excerpt occurs on the declared source page; diagnostics are converted to safe bounded values.
- Server normalization uses exact rationals and `money-v1` ties-to-even rounding for USD/EUR/GBP/RUB/JPY, strict ISO dates, bounded quantities, and server-generated arithmetic warnings. Raw candidate and normalized data, warnings, evidence, diagnostics, and policy version are immutable database snapshot fields.
- Docker Compose now carries the pinned Poppler/Tesseract runtime. `scripts/compose-smoke.sh` verifies upload → worker extraction → immutable version/`needs_review` against the fictional embedded-text fixture.
- `GET /api/v1/documents/{id}` is a bounded, no-store document-detail representation: it returns up to 100 immutable versions, their proposal/normalized/warning/evidence/diagnostic fields, exact-decimal editable values, and up to 100 audit events without leaking original filenames, storage keys, paths, or hashes.
- `GET /api/v1/documents/{id}/source` looks up the server-owned original and streams only its stored PDF/JPEG/PNG media bytes with inline/no-store/nosniff headers.
- A strict human-edit endpoint accepts candidate metadata, dates, currency, money, and line items only. The server applies the existing `money-v1` policy and current normalizer, locks the document to serialize version allocation, atomically appends a `human_review` snapshot and `human_review_saved` audit, and never mutates extraction data. Historical extraction warnings remain preserved in the extraction snapshot; the human version receives a fresh server-generated warning set and carries source evidence/sanitized diagnostics forward.
- A confirmed reject endpoint atomically transitions only `needs_review` to terminal `rejected` and appends `document_rejected`; it preserves source and all immutable snapshots.
- `/app/documents/{id}` is an accessible responsive split review UI with the original on the left and proposal/edit form, line items, warnings, evidence, diagnostics, and audit history on the right. It has real loading, empty, failed/reload, unsaved-change, save conflict/error, confirmation, and rejected read-only states.

## Deliberately not implemented

There is no approval, CSV/webhook export, payment, product-media work, final landing scroll sequence, document list, or job retry endpoint. There is no production PDF sandbox, malware scanner, OCR support for scanned PDFs, live provider, user authentication, or claim of extraction accuracy.

## Stage 4 validation

Passed on 2026-07-23:

- `go test ./... && go vet ./...`.
- `DATABASE_URL=postgres://invoiceflow:invoiceflow@127.0.0.1:5432/invoiceflow?sslmode=disable go test -tags=integration ./...` — including isolated human-review immutability, stale-version, and rejection transaction coverage.
- `cd web && npm run test && npm run build`.
- `COMPOSE_PROJECT_NAME=invoiceflowstage4 API_HOST_PORT=8083 POSTGRES_HOST_PORT=5435 docker compose up --build --wait --force-recreate` followed by `sh scripts/compose-smoke.sh` — health/readiness, fictional upload/extraction, source access, `human_review` version creation, audit events, and terminal rejection passed. The temporary test project and volumes were then removed.

## Risks carried forward

- Package/base image pins need an intentional update process; the runtime is bounded but not a kernel sandbox.
- PDF raster OCR needs a dedicated implementation with per-render page/pixel accounting before it is enabled.
- The local demo is fixed-actor/no-auth. Source delivery is intentionally document-scoped but needs real authentication/authorization before a production deployment.
- Review detail has fixed history bounds, not pagination; document list/search and manual processing retry remain future scope.

## Exact next prompt

> Implement Stage 5 only: add explicit approval of an exact immutable review version, CSV export, and signed generic webhook export. Do not add payment, live provider, product media, or final landing-scroll work.
