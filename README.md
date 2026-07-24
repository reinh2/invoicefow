# InvoiceFlow

InvoiceFlow turns a PDF, JPEG, or PNG invoice into normalized, versioned business data — and stops there. Extraction produces a **proposal**, not an answer. A person compares it against the original, corrects it, and approves one exact version before anything can be exported.

It is not an accounting system. It never pays an invoice and never connects to a bank.

> **Status:** working end-to-end demo, not a released product. Stages 0–5 are complete; Stage 6 (product presentation) is partially complete. See [Honest limitations](#honest-limitations) and [`docs/CURRENT_TASK.md`](docs/CURRENT_TASK.md).

## Why it exists

Invoice data arrives as files and gets retyped into spreadsheets and accounting tools. Manual entry is slow and error-prone; fully autonomous extraction is risky, because financial documents vary and mistakes are expensive.

InvoiceFlow keeps the speed and the control:

- the original document stays visible next to the extracted values;
- the server says what it could not verify, in explicit warnings;
- every correction creates a new immutable version instead of overwriting one;
- approval targets one exact version number and requires confirmation;
- export reads only the approved version, and repeats are idempotent;
- the whole history is append-only.

## Quick start

Requires Docker. No paid credentials, no API key, no network calls to a model provider.

```bash
docker compose up --build --wait
```

Then open <http://127.0.0.1:8080> and upload `testdata/stage2-fictional-compose.pdf` from the repository.

**The offline extractor only recognizes the bundled fictional fixtures.** It matches on the server-computed SHA-256 plus an embedded marker, so uploading an arbitrary invoice is accepted and processed, but returns an empty proposal with a diagnostic rather than invented values. That is deliberate: the default demo never guesses.

To drive the full flow from the command line instead:

```bash
sh scripts/demo-seed.sh
```

It uploads two fictional documents, saves a correction on one, and carries the other through approval, CSV export, and a signed webhook delivery — using only the public API.

## Demo flow

1. **Upload** — one PDF, JPEG, or PNG up to 20 MiB. The extension, the declared media type, and the file signature must agree.
2. **Process** — a durable PostgreSQL job extracts text under fixed page, byte, and time bounds, falling back to OCR for images.
3. **Review** — the original renders on the left, editable normalized values on the right, with server warnings, source evidence, and the audit trail.
4. **Correct** — saving writes a new immutable version; the previous one keeps its own values and warnings.
5. **Approve** — an explicit version number plus a confirmation. The document becomes read-only for review edits.
6. **Export** — CSV of the approved snapshot (byte-identical on every repeat), or a signed webhook delivery with a stable idempotency key.

Document states: `uploaded`, `queued`, `processing`, `needs_review`, `approved`, `rejected`, `exported`, `failed`. Invalid transitions return a stable error.

## Architecture

A Go modular monolith with separate API and worker executables, PostgreSQL, and a React/TypeScript interface.

```text
React + Vite (web/)              Go API (cmd/api)
  /      product story             GET  /healthz, /readyz
  /app   upload + split review     POST /api/v1/documents
                                   GET  /api/v1/documents/{id}
                                        …review, approve, export
                                          |
              validate → SHA-256 → private temp file → promote
                                          |
        one transaction: stored object + document + audit event + job
                                          |
                               Go worker (cmd/worker)
        claim lease → bounded PDF text → OCR fallback → strict proposal
              → normalize + warn → immutable version → needs_review
```

- **Durable work.** Processing and export are PostgreSQL jobs with lease tokens, recorded attempts, expired-lease recovery, bounded retries, and dead-letter states. Restarting a worker does not lose queued work.
- **Exact money.** Amounts are integer minor units with an explicit currency, under a named rounding policy (`money-v1`) stored on every snapshot. No binary floating point touches an amount.
- **Immutability.** Invoice versions and audit events reject updates and deletes at the database level.
- **Replaceable adapters.** Storage, PDF, OCR, and the structured extractor sit behind interfaces. The default extractor is deterministic and offline.

Details: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), [`docs/API_CONTRACT.md`](docs/API_CONTRACT.md), and the decision log in [`docs/DECISIONS.md`](docs/DECISIONS.md).

## Security model

Uploaded files, extracted text, OCR output, and model output are all treated as untrusted input. None of them can set a document state, a storage key, an actor, an approval, or an export destination.

- Uploads are bounded and validated by extension, parsed media type, and file signature; images are fully decoded within a pixel ceiling.
- Storage keys are generated server-side. Client filenames are used only for extension validation and are never persisted, logged, or returned.
- Extraction tools are invoked as fixed absolute paths with literal argument arrays under process timeouts and output caps. No filename is ever interpolated into a shell string.
- Webhook destinations and secrets are process configuration, never request data. Strict mode is HTTPS-only, redirect-free, port 443, and rejects private and reserved addresses with DNS-answer validation.
- The browser bundle is served from memory by exact key lookup, so path traversal and symlink escape are structurally impossible rather than filtered. Every static response carries a fixed first-party CSP with no `unsafe-inline` or `unsafe-eval`.
- Logs and provider errors are sanitized. No secret, raw payload, storage path, or document text is logged.

Full model: [`docs/SECURITY_MODEL.md`](docs/SECURITY_MODEL.md).

## Commands

```bash
make fmt              # gofmt
make lint             # go vet
make test             # Go unit tests
make test-integration # PostgreSQL integration tests, requires DATABASE_URL
make frontend-test    # typecheck, Vitest, production build
make build            # both binaries and the web bundle
make smoke-compose    # full Compose smoke: upload → … → export → audit
```

Local development without Docker needs a reachable PostgreSQL, then `go run ./cmd/api`, `go run ./cmd/worker`, and `npm run dev` in `web/` (Vite proxies `/api`).

Configuration is environment-only; see [`.env.example`](.env.example). `WEB_DIR` is optional — when it is empty the API serves JSON only.

## Honest limitations

These are current boundaries of the running system, not a roadmap.

- **No payments.** No invoice payment, bank connectivity, or autonomous financial approval, by design.
- **Not accounting.** No bookkeeping, double-entry, or tax filing, and no compliance claim of any kind.
- **No authentication.** The local demo uses one fixed server-side actor. There is no login and no multi-user authorization. Production access control belongs at this boundary and is not implemented.
- **No live model provider.** The default extractor is a deterministic offline fake. A paid provider adapter would have to implement the same strict schema and server-side validation.
- **OCR covers images only.** JPEG and PNG go through Tesseract. Raster OCR for scanned PDFs is deliberately not implemented rather than silently approximated.
- **Webhook delivery is at-least-once.** Receivers must deduplicate by the idempotency key. "Exactly once" is not claimed.
- **No document list or search**, no manual retry endpoint, and no metrics endpoint.
- **No screenshots or demo media yet.** They are a Stage 6/7 deliverable and will be captured from the real application; nothing here is mocked up in their place.
- **The bundled fixtures are minimal synthetic files.** They exercise the pipeline but do not look like realistic invoices yet.
- The Compose bootstrap role serves both migrations and runtime, so this is not a least-privilege deployment.

No metric, customer, accuracy rate, or certification on this page is asserted, because none has been measured.

## Testing

- Go unit tests: validation, hashing and duplicates, state transitions, money and date normalization, schema validation, arithmetic warnings, retry classification, webhook signatures, export idempotency, and static delivery.
- PostgreSQL integration tests: migrations and their guard rails, the atomic intake transaction, concurrent duplicate handling, single-winner job claims, lease recovery and attempt history, immutable review versions, orphan reconciliation, export retry and dead-lettering, and the composite foreign keys that bind an export to its exact approved version.
- Frontend tests: upload states, extracted/warning/edit presentation, line-item editing, approval confirmation, export lifecycle, reduced motion, accessible labels, and an axe pass.
- Compose smoke: start, readiness, upload, processing, review, correction, approval, idempotent CSV, signed webhook delivery, audit events, and rejection.

CI runs the Go, integration, and frontend suites on every push and pull request.

## License

No license file is present yet, so no permission to reuse this code is granted.
