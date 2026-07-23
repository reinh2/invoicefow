# Architecture

## Implemented through Stage 4

InvoiceFlow is a Go modular monolith with a React/Vite interface, separate API and worker executables, PostgreSQL, and server-owned filesystem storage for its local demo.

```text
React/Vite (`web/`)                  Go API (`cmd/api`)
  / marketing shell                    /healthz, /readyz
  /app upload and split review         POST /api/v1/documents
            |                                      |
            +------------ development proxy -------+
                                                   |
                         validate → hash → private temporary file
                                                   |
                         filesystem promotion (`/data/objects`)
                                                   |
                    PostgreSQL transaction: object + document + audit + job
                                                   |
                              Go worker (`cmd/worker`)
              claim lease → bounded PDF text → OCR fallback → fake proposal
                    normalize/warn → immutable extraction version → needs_review
```

The API and worker load server-side configuration, open bounded `pgxpool` connections, take a PostgreSQL advisory lock for forward checksummed migrations, and use UTC timestamps. The API defaults to loopback `127.0.0.1:8080`; Compose publishes it only to loopback. The fixed `local-demo` audit actor is server configuration, not client input.

### Intake and original storage

`internal/documents` streams one multipart file to a private temporary file while calculating SHA-256. It enforces a 20 MiB limit, extension/declared-media-type/signature agreement for PDF/JPEG/PNG, a PDF end-marker gate, and bounded full standard-library image decoding. This does not mean PDFs are trusted: the worker repeats semantic/encryption validation with the pinned Stage 3 toolchain before a PDF text process consumes stored bytes. Browser filenames are used only for extension validation; they are not persisted, logged, or returned.

`internal/platform.FileStorage` accepts only server-generated `objects/<32 lowercase hex>.<pdf|jpg|jpeg|png>` keys. It rejects unsafe roots/directories and final-file symlinks, writes to a private `tmp` directory, fsyncs, and promotes with no-overwrite hard-link semantics. It never joins client path material. The configured storage root and its parent path are trusted local Compose configuration; a shared or attacker-writeable volume is not a supported deployment.

The API stores bytes before the database transaction, deletes its newly promoted object when that transaction fails, and never creates a job before storage exists. The worker runs startup and five-minute maintenance: it removes only aged private temporary files, removes only aged unreferenced server-shaped objects, and recovers expired leases. A crash after promotion can temporarily leave an unreferenced object during the 24-hour grace window, but cannot create a job that points to absent storage.

### Database and durable jobs

The schema contains `stored_objects`, `documents`, `jobs`, `job_attempts`, `invoice_versions`, and append-only `audit_events`.

- Stored-object key/hash identity is immutable; the size is positive and media type is constrained to the Stage 2 accepted types.
- `documents.sha256` is globally unique and trigger-checked against its stored object. A duplicate response is intentionally opaque.
- One intake transaction inserts the stored-object record, queued document, first ordered audit event, and one `process_document` job.
- Repository primitives use `FOR UPDATE SKIP LOCKED`, opaque lease tokens, one open attempt, heartbeat, bounded retry/dead-letter, and expired-lease recovery. Each document transition and its audit event are atomic.
- The worker claims one `process_document` job at a time under the existing lease protocol. It reopens the immutable original from storage, performs bounded extraction outside the database transaction, and atomically writes an extraction version, closes the attempt, and moves `processing` to `needs_review`. Retryable infrastructure failures use the bounded retry protocol; encrypted/malformed PDFs and contract/limit violations dead-letter immediately with generic summaries.
- `invoice_versions` holds immutable, bounded raw proposal JSON, normalized JSON, warnings, evidence, sanitized diagnostics, and the rounding-policy version. Stage 4 reads at most the latest 100 snapshots for one document and returns exact-decimal editable values rather than browser floating-point money.
- A Stage 4 human edit locks the document row, confirms `needs_review`, compares its base version to the latest immutable version, normalizes strict candidate strings on the server, then atomically writes `source='human_review'` and a `human_review_saved` audit event. Evidence/diagnostics remain source context; a new warning set is server-generated for the new candidate, while prior warnings remain in the immutable prior version. The lock serializes version allocation.
- A confirmed Stage 4 rejection locks the same row, atomically moves `needs_review` to terminal `rejected`, and appends `document_rejected`; it creates no new invoice version.

### Extraction trust boundary

The final image includes the ADR-006 pinned Poppler and Tesseract packages. `pdfinfo` semantically gates PDFs and enforces the 50-page bound before `pdftotext` is allowed to consume a private temporary copy. Process contexts, fixed absolute executable paths/literal argument arrays, a 512 KiB stdout/stderr cap, 15-second PDF deadline, and 256 KiB reference-text cap bound this path. Empty text invokes the OCR port; the shipped Tesseract adapter supports revalidated JPEG/PNG inputs under the 40-million-pixel limit. PDF raster OCR is deliberately deferred, not silently emulated.

The deterministic no-network fake extractor requires both the server-computed hash of the fictional Compose fixture and its embedded marker. Its output is passed through strict-schema/evidence validation and exact server normalization; it cannot set a workflow state, storage location, authority, or export target. Evidence is persisted only when its quoted excerpt occurs on the declared extracted page; diagnostics are reduced to safe generic values.
- Audit events and invoice versions reject mutation; stored-object identity is immutable. The Compose bootstrap role still serves migration and runtime, so this is not a least-privilege production deployment.

### Frontend and review surface

The `/` marketing route and `/app` application route use Calibrated Ledger tokens: navy/near-black operational surfaces, warm-paper source panels, restrained blue process/focus, amber warnings, and green terminal states. A successful upload opens `/app/documents/{id}`. The route fetches a real bounded review representation and renders the original from a document-scoped no-store stream on the left, with editable normalized candidate strings, warnings, source evidence, sanitized diagnostics, and audit history on the right. Saving adds a new immutable human-review version; rejected documents remain read-only. The UI has loading, empty, failed/reload, unsaved-change, modal-confirmation, and keyboard-accessible native control states. It contains no fabricated extraction data.

The source stream performs a document lookup before opening a server-owned storage key and returns only stored media bytes with inline/no-store/nosniff response headers. It does not expose filenames, object keys, hashes, or paths. The local demo remains fixed-actor/no-auth; production authorization must sit at this boundary.

Motion is CSS transform/opacity utility work with global `prefers-reduced-motion` behavior. Vite forwards `/api` in local development; production frontend serving is not packaged yet.

### Deferred stages

Stage 5 adds explicit approval, CSV, and signed webhook export. Stage 6 uses live DOM for the main scroll story and must also ship one optimized factual product-media element from the real fictional application (screen recording, WebM/MP4 with poster/fallback, or image sequence). Stage 7 verifies the portfolio release.
