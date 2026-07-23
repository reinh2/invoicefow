# Architecture Decision Log

## ADR-000 — Initial direction

**Status:** Proposed

**Context:** The project must demonstrate reliable document processing without becoming an accounting platform.

**Decision:** Start from a Go modular monolith with separate API/worker executables, PostgreSQL-backed durable jobs, React/Vite frontend, storage/PDF/OCR/provider interfaces, and a deterministic no-key demo.

**Consequences:**

- The system remains understandable as a portfolio project.
- Durable work survives process restarts.
- External tools remain replaceable.
- Horizontal scaling and production compliance are not initial claims.

## ADR-001 — Stage 0 architecture and durable-work baseline

**Status:** Proposed — pending Stage 1 approval

**Context:** Repository inspection found an instruction-only scaffold with no existing implementation to preserve.

**Decision:** Build a Go modular monolith with separate API and worker binaries, PostgreSQL/pgx, forward SQL migrations protected by an advisory lock, and database-backed at-least-once processing/export jobs with lease tokens, bounded retries, dead-letter states, and stable external idempotency keys. Use immutable version snapshots, integer minor-unit money plus currency, server-generated UUIDs/storage keys, and append-only audit events. A storage-promotion/orphan-reconciliation protocol handles the unavoidable boundary between object storage and PostgreSQL.

**Consequences:** The default demo remains simple and restart-safe. Webhooks cannot be claimed exactly once; their receiver must deduplicate by the stable idempotency key.

## ADR-002 — Calibrated Ledger visual and motion direction

**Status:** Proposed — pending Stage 1 approval

**Context:** InvoiceFlow must present a premium SaaS identity without imitating an admin template, cyberpunk aesthetic, or autonomous-AI product.

**Decision:** Use a near-black/navy operational interface with warm paper source-document panels, restrained blue processing/focus, amber warnings, green approved/exported states, and thin evidence rails connecting source, proposal, correction, and export. Define a shared CSS-variable token layer and a reduced-motion-aware motion utility. Use live DOM for the scroll story; defer recorded demo media until real fictional flows exist.

**Consequences:** Both marketing and application screens share one honest visual language. No final video asset, remote font, animation package, fake metric, or unsupported completion claim is introduced in the foundation stage.

## ADR-003 — Default demo authority and webhook boundary

**Status:** Proposed — pending Stage 1 approval

**Context:** Approval/export need server-side authority, while the no-key demo must remain local and simple. User-controlled webhook destinations would create SSRF and replay risk.

**Decision:** Default mode is a loopback-bound local demo with a server-configured fixed actor; it makes no multi-user authorization claim. Audit actors are server-derived, never trusted from JSON. Webhooks are disabled by default; when Stage 5 introduces them, destination and secret are server-managed, HTTPS-only, redirect-free, and protected against private/reserved-address access, replay, and duplicate delivery.

**Consequences:** A multi-user authentication/authorization system remains a separately designed future capability. Approval/export APIs must enforce the chosen server-side authority before they are exposed.

Use `templates/ADR_TEMPLATE.md` for new decisions.
