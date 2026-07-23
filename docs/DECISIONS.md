# Architecture Decision Log

## ADR-000 — Initial direction

**Status:** Accepted

**Context:** The project must demonstrate reliable document processing without becoming an accounting platform.

**Decision:** Start from a Go modular monolith with separate API/worker executables, PostgreSQL-backed durable jobs, React/Vite frontend, storage/PDF/OCR/provider interfaces, and a deterministic no-key demo.

**Consequences:**

- The system remains understandable as a portfolio project.
- Durable work survives process restarts.
- External tools remain replaceable.
- Horizontal scaling and production compliance are not initial claims.

## ADR-001 — Stage 0 architecture and durable-work baseline

**Status:** Accepted

**Context:** Repository inspection found an instruction-only scaffold with no existing implementation to preserve.

**Decision:** Build a Go modular monolith with separate API and worker binaries, PostgreSQL/pgx, forward SQL migrations protected by an advisory lock, and database-backed at-least-once processing/export jobs with lease tokens, bounded retries, dead-letter states, and stable external idempotency keys. Use immutable version snapshots, integer minor-unit money plus currency, server-generated UUIDs/storage keys, and append-only audit events. A storage-promotion/orphan-reconciliation protocol handles the unavoidable boundary between object storage and PostgreSQL.

**Consequences:** The default demo remains simple and restart-safe. Webhooks cannot be claimed exactly once; their receiver must deduplicate by the stable idempotency key.

## ADR-002 — Calibrated Ledger visual and motion direction

**Status:** Accepted

**Context:** InvoiceFlow must present a premium SaaS identity without imitating an admin template, cyberpunk aesthetic, or autonomous-AI product.

**Decision:** Use a near-black/navy operational interface with warm paper source-document panels, restrained blue processing/focus, amber warnings, green approved/exported states, and thin evidence rails connecting source, proposal, correction, and export. Define a shared CSS-variable token layer and a reduced-motion-aware motion utility. Use live DOM for the scroll story; defer recorded demo media until real fictional flows exist.

**Consequences:** Both marketing and application screens share one honest visual language. No final video asset, remote font, animation package, fake metric, or unsupported completion claim is introduced in the foundation stage.

## ADR-003 — Default demo authority and webhook boundary

**Status:** Accepted

**Context:** Approval/export need server-side authority, while the no-key demo must remain local and simple. User-controlled webhook destinations would create SSRF and replay risk.

**Decision:** Default mode is a loopback-bound local demo with a server-configured fixed actor; it makes no multi-user authorization claim. Audit actors are server-derived, never trusted from JSON. Webhooks are disabled by default; when Stage 5 introduces them, destination and secret are server-managed, HTTPS-only, redirect-free, and protected against private/reserved-address access, replay, and duplicate delivery.

**Consequences:** A multi-user authentication/authorization system remains a separately designed future capability. Approval/export APIs must enforce the chosen server-side authority before they are exposed.

## ADR-004 — Stage 1 database foundation and immutability

**Status:** Accepted

**Context:** The foundation needs durable schema constraints before upload or worker workflows are introduced.

**Decision:** Use checksummed, transactionally applied, advisory-lock-protected PostgreSQL migrations. Keep stored-object identity immutable; enforce document/object hash equality; store exact money as `BIGINT` plus currency; reject update/delete of invoice-version snapshots; and reject update/delete/truncate of audit events. Reject retroactively inserted migration names that sort before an applied migration.

**Consequences:** The Stage 1 schema is safe to build later workflows on. The Compose bootstrap role is still shared by migration and runtime connections, so least-privilege role separation remains a documented hardening item rather than a current claim.

## ADR-005 — Real product media remains a release requirement

**Status:** Accepted

**Context:** Live DOM is the primary scroll-driven story, but the portfolio also requires an actual optimized media element created from the real application.

**Decision:** Do not fabricate or create final media in Stage 1. By Stage 6 or, at the latest, Stage 7, ship at least one factual fictional-demo asset: a product screen recording, optimized muted WebM/MP4 with poster/fallback, or an image sequence generated from the real UI. Document its regeneration and retain static/mobile/no-video fallbacks.

**Consequences:** The landing page remains honest during development while the portfolio release still demonstrates media production and performance discipline.

## ADR-006 — Pinned PDF/OCR toolchain, limits, and hostile-PDF handling

**Status:** Accepted (Stage 3)

**Context:** PDF and OCR parsing are native-tool attack surfaces. Intake's header/end-marker gate is deliberately not a semantic PDF validation.

**Decision:** The local demo image pins Debian 12.11 and its named packages `poppler-utils` 22.12.0-2+deb12u2 (Poppler command-line utilities, GPL-2.0-or-later) and `tesseract-ocr` 5.3.0-2 (Apache-2.0). The worker invokes only `/usr/bin/pdfinfo`, `/usr/bin/pdftotext`, and `/usr/bin/tesseract` through literal argument arrays; client names and storage paths are never command arguments. The Stage 3 default uses Poppler `pdfinfo` before text extraction; its page count is authoritative for the configured bound and an encrypted result is a permanent rejection. Any failed semantic inspection or text parse is treated as a permanent malformed-PDF failure, without exposing tool output.

The server accepts at most 20 MiB as in Stage 2, extracts at most 50 pages, accepts raster pages only through the existing 10,000-by-10,000 / 40,000,000-pixel ceiling, passes at most 256 KiB of reference text to a provider, accepts at most 64 KiB of provider output, and caps any individual tool stdout/stderr at 512 KiB. PDF text inspection/extraction gets 15 seconds total; OCR gets 30 seconds total. A process context kills an overrun; bounded writers stop output amplification. The worker writes a server-owned temporary copy in the private storage temporary directory and deletes it after each command. This is process isolation and bounded I/O, not a claim of a kernel sandbox or antivirus scanning.

OCR is a replaceable port. The default adapter runs Tesseract for validated JPEG/PNG inputs; its PDF-raster implementation is intentionally deferred until it can account for every rendered page's pixels. OCR is attempted only after bounded text extraction returns no non-whitespace text. The no-key fake provider remains the default structured extractor and makes no network call.

**Consequences:** The runtime image is deliberately not distroless because it needs the pinned parser executables. Production deployments need a separately reviewed OS/package update process and stronger sandboxing; Stage 3 does not claim either.

## ADR-007 — Versioned exact normalization; webhook contracts remain deferred

**Status:** Partially accepted (Stage 3); webhook section deferred to Stage 5

**Context:** Exact domain storage exists, but decimal conversion/rounding and date normalization need stable replayable semantics.

**Decision:** Persist `rounding_policy_version = "money-v1"` with every extraction snapshot. Version 1 accepts only ASCII base-10 decimal strings with an optional leading minus and no grouping, currency signs, exponent notation, or binary floating point. It supports ISO currency exponents USD/EUR/GBP/RUB = 2 and JPY = 0. Values are converted with `math/big.Rat`; any fractional minor unit rounds to nearest with ties-to-even. Quantity is an exact decimal with at most six fractional digits. Dates accept only trimmed ISO `YYYY-MM-DD`, validate in UTC, and are stored canonically as that date; no locale guessing occurs. Missing/invalid values remain absent and become warnings, never invented zeroes.

The normalizer records the raw proposal alongside canonical values and stable server-generated warnings. It recomputes a line from quantity × unit price + line tax when all are available, compares line totals, compares the line sum to subtotal/total where enough data exists, and compares subtotal + tax to total. It does not assert tax or accounting correctness. A future policy change requires a new named version and never mutates a historical snapshot.

Before Stage 5, define server-managed destination/secret storage, HMAC canonical bytes, timestamps, replay window, redirect policy, and SSRF/DNS-rebinding controls.

## ADR-010 — Strict proposal schema, evidence, and diagnostic boundary

**Status:** Accepted (Stage 3)

**Context:** Provider output and source text are untrusted reference data. Typed Go values alone do not make a provider response safe or sufficiently specified.

**Decision:** The provider contract is a strict JSON object decoded with unknown fields rejected. It permits only nullable string candidates for supplier metadata, dates, currency, subtotal/tax/total, bounded line-item candidates, evidence, and diagnostics; numbers, confidence, IDs, status, storage keys, URLs, instruction fields, approvals, and export fields are not part of the schema. Server limits apply before normalization.

Evidence is optional and has only a declared invoice field, one-based source page, a literal bounded excerpt, and optional adapter-coordinate bounding box. The server persists evidence only when its field is recognized, its page exists, and its excerpt occurs in that exact bounded source page; it never synthesizes evidence or gives a bounding box a rendering-coordinate meaning. Diagnostics are reduced to a bounded allowlisted code and generic safe message before persistence. Raw provider responses, stderr, filenames, storage paths, prompts, and secrets are neither stored nor exposed.

**Consequences:** Extraction snapshots are useful for review without making provider output authoritative. A future provider adapter must explicitly implement this schema and cannot bypass server validation.

## ADR-011 — Immutable human review, rejection, and original-document delivery

**Status:** Accepted (Stage 4)

**Context:** Stage 3 leaves each document in `needs_review` with one immutable extraction snapshot. Review needs an editable proposal, but neither an edit nor a browser request may change extraction evidence, provider diagnostics, original bytes, or historical values.

**Decision:** A review edit is a strict, bounded candidate proposal submitted only while a document is `needs_review`. The server reuses `money-v1` and the existing date/arithmetic normalizer, preserves evidence and sanitized diagnostics from the latest immutable snapshot, and atomically inserts a new `source='human_review'` version plus a `human_review_saved` audit event. Existing versions are never updated. Rejection is a confirmed client action that atomically changes only `needs_review` to `rejected` and appends `document_rejected`; it creates no version and is terminal in Stage 4. Stale edits and invalid reject transitions return stable `409 invalid_document_transition` errors.

The document detail route returns bounded, presentation-safe metadata, up to the latest 100 immutable review versions and 100 latest audit events, with the newest version selected by highest version number. Original bytes are available only from a document-scoped `GET` route after repository lookup of the server-owned key; it streams the configured stored media type with `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, and `Content-Disposition: inline`. It exposes neither filenames, storage keys, filesystem paths, hashes, nor database internals. The local demo has no user authorization claim; production access control remains deferred.

**Consequences:** The UI can compare a human correction with the original extraction, while the audit trail remains append-only and review data cannot become authoritative without a future approval stage. Stored originals remain private except for an intentionally bounded review representation. Stage 4 has no approval, export, payment, list, retry, or authenticated multi-user API.

Use `templates/ADR_TEMPLATE.md` for new decisions.

## ADR-008 — Stage 2 intake, duplicate, and storage boundary

**Status:** Accepted

**Context:** Original bytes must exist before a document/job does, but filesystem storage and PostgreSQL cannot share one transaction. Browser-provided names, media types, and content are untrusted.

**Decision:** Accept one multipart `file` within a 21 MiB request/20 MiB file bound. Require extension, parsed declared media type, and signature agreement; require a PDF end marker and a bounded full image decode. Stream SHA-256 to private temporary storage, promote under an opaque server key, then atomically create the stored object, queued document, upload audit event, and process job. SHA-256 is globally unique and yields an opaque duplicate conflict. On transaction failure, attempt immediate cleanup; worker maintenance removes only aged private temporary files and unreferenced server-shaped objects.

**Consequences:** A crash can retain unreferenced bytes only until the 24-hour grace period; it cannot create a job pointing at missing storage. PDF semantics/encryption are not validated yet and must be revalidated before Stage 3 process execution.

## ADR-009 — Stage 2 durable process-job primitives

**Status:** Accepted

**Context:** Intake needs stable concurrency/retry semantics before PDF/OCR work starts.

**Decision:** Create one `process_document` job per document. Claim with `FOR UPDATE SKIP LOCKED`, use opaque lease tokens, permit one open attempt, and atomically update document state plus audit. Heartbeats require a matching unexpired token. Retry and expired-lease recovery close attempts, preserve bounded error summaries, schedule retry, and dead-letter at maximum attempts.

**Consequences:** The repository and isolated PostgreSQL tests prove the protocol. The worker runs lease recovery but does not claim/extract jobs until Stage 3 resolves tool and trust-boundary ADRs.
