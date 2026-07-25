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

**Fulfilled (Stage 6):** `web/public/media/` ships `demo.webm` (a muted landing screen capture), `demo-landing-poster.png` (poster), and `demo-review.png` (the split review screen on the seeded Meridian fixture, doubling as the reduced-motion and no-video fallback). All three are captured from the real application on fictional fixtures by the committed `web/scripts/capture-media.mjs` (`@playwright/test`) against an isolated seeded Compose demo; the landing's `DemoMedia` component embeds the video with the still as its reduced-motion path.

## ADR-006 — Pinned PDF/OCR toolchain, limits, and hostile-PDF handling

**Status:** Accepted (Stage 3)

**Context:** PDF and OCR parsing are native-tool attack surfaces. Intake's header/end-marker gate is deliberately not a semantic PDF validation.

**Decision:** The local demo image pins Debian 12.11 and its named packages `poppler-utils` 22.12.0-2+deb12u2 (Poppler command-line utilities, GPL-2.0-or-later) and `tesseract-ocr` 5.3.0-2 (Apache-2.0). The worker invokes only `/usr/bin/pdfinfo`, `/usr/bin/pdftotext`, and `/usr/bin/tesseract` through literal argument arrays; client names and storage paths are never command arguments. The Stage 3 default uses Poppler `pdfinfo` before text extraction; its page count is authoritative for the configured bound and an encrypted result is a permanent rejection. Any failed semantic inspection or text parse is treated as a permanent malformed-PDF failure, without exposing tool output.

The server accepts at most 20 MiB as in Stage 2, extracts at most 50 pages, accepts raster pages only through the existing 10,000-by-10,000 / 40,000,000-pixel ceiling, passes at most 256 KiB of reference text to a provider, accepts at most 64 KiB of provider output, and caps any individual tool stdout/stderr at 512 KiB. PDF text inspection/extraction gets 15 seconds total; OCR gets 30 seconds total. A process context kills an overrun; bounded writers stop output amplification. The worker writes a server-owned temporary copy in the private storage temporary directory and deletes it after each command. This is process isolation and bounded I/O, not a claim of a kernel sandbox or antivirus scanning.

OCR is a replaceable port. The default adapter runs Tesseract for validated JPEG/PNG inputs; its PDF-raster implementation is intentionally deferred until it can account for every rendered page's pixels. OCR is attempted only after bounded text extraction returns no non-whitespace text. The no-key fake provider remains the default structured extractor and makes no network call.

**Consequences:** The runtime image is deliberately not distroless because it needs the pinned parser executables. Production deployments need a separately reviewed OS/package update process and stronger sandboxing; Stage 3 does not claim either.

**Amendment (Stage 8, ADR-015):** `pdftotext` is invoked with an additional fixed literal `-layout` argument. Without it Poppler emits column-major text that separates every invoice label from its amount, degrading the reference text for any extractor. Bounds, timeout, and the argument-array invocation are unchanged.

## ADR-007 — Versioned exact normalization; webhook contracts remain deferred

**Status:** Partially accepted (Stage 3); webhook section deferred to Stage 5

**Context:** Exact domain storage exists, but decimal conversion/rounding and date normalization need stable replayable semantics.

**Decision:** Persist `rounding_policy_version = "money-v1"` with every extraction snapshot. Version 1 accepts only ASCII base-10 decimal strings with an optional leading minus and no grouping, currency signs, exponent notation, or binary floating point. It supports ISO currency exponents USD/EUR/GBP/RUB = 2 and JPY = 0. Values are converted with `math/big.Rat`; any fractional minor unit rounds to nearest with ties-to-even. Quantity is an exact decimal with at most six fractional digits. Dates accept only trimmed ISO `YYYY-MM-DD`, validate in UTC, and are stored canonically as that date; no locale guessing occurs. Missing/invalid values remain absent and become warnings, never invented zeroes.

The normalizer records the raw proposal alongside canonical values and stable server-generated warnings. It recomputes a line from quantity × unit price + line tax when all are available, compares line totals, compares the line sum to subtotal/total where enough data exists, and compares subtotal + tax to total. It does not assert tax or accounting correctness. A future policy change requires a new named version and never mutates a historical snapshot.

Stage 5's ADR-012 defines the server-managed destination/secret boundary, HMAC canonical bytes, timestamp window, redirect policy, and SSRF/DNS-rebinding controls.

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

## ADR-012 — Stage 5 explicit version approval, CSV export, and generic signed webhooks

**Status:** Accepted

**Context:** Stage 4 provides immutable review versions and rejection, but cannot finalize an invoice or export structured data. Approval must target an explicit version, lock the document against further review edits, and enable idempotent export to CSV and signed generic webhooks without introducing financial payments, unverified external URLs, or unsafe SSRF vulnerabilities.

**Decision:** Approval is an explicit POST request requiring `version_number` and `confirm: true`. The server transactionally locks the document, verifies `status = 'needs_review'`, checks that the specified version is the current immutable version, sets `approved_version_id` and `approved_at`, updates status to `approved`, and appends a versioned `document_approved` audit event. Approved documents become read-only for review edits.

CSV export (`GET /api/v1/documents/{id}/export/csv`) is permitted only when status is `approved` or `exported`. Public format `csv-v1` is UTF-8, RFC 4180 escaped, uses CRLF record endings, fixed columns/order, and exact server-normalized decimal money strings (never browser floats). The response filename is `invoice-{document_id}-v{version}.csv`. The server reads only the immutable approved foreign key, atomically records one export row, moves `approved` to `exported` on first export, and returns deterministic bytes without duplicating audit events on repetition.

Webhook export (`POST /api/v1/documents/{id}/export/webhook`) creates one `export_document` job and `exports` record for the exact approved version. The record stores only `server:webhook:v1`, a safe label, stable idempotency key, status, attempt projection, and safe error. The URL and secret remain process configuration. The worker builds canonical JSON, signs `timestamp + "." + body` with HMAC-SHA256, and sends `X-InvoiceFlow-Signature`, `X-InvoiceFlow-Timestamp`, and `X-InvoiceFlow-Idempotency-Key`. Strict mode is the default: HTTPS, port 443, no redirects, no userinfo/query/fragment, private/reserved DNS answers denied, DNS dial pinned to a validated answer, bounded timeout and response body. The Compose-only controlled adapter is an exact fixed receiver destination, not a private-network bypass.

The receiver contract uses a five-minute timestamp window and constant-time `hmac.Equal` verification. Retries reuse the same idempotency key and canonical body; the receiver deduplicates the key and rejects reuse with different bytes. Expired export leases close attempts and append safe retry/dead-letter audit events without changing the approved document state.

The persisted `exports.idempotency_key` is the sole delivery key: it is returned by the API, loaded by the worker, placed in the canonical payload, sent in `X-InvoiceFlow-Idempotency-Key`, and used by the controlled receiver for deduplication. The public export projection exposes only `version_number`; the internal version UUID remains server-side. Forward migrations add one composite foreign key from `(exports.document_id, exports.version_id)` to the matching invoice-version document pair and another from that pair to `(documents.id, documents.approved_version_id)`. Thus a direct writer cannot create an export for a different historical version of the same document. On every retry, success, permanent failure, lease recovery, or retry exhaustion the export row receives the claimed job's exact attempt count; terminal jobs and exports have no next-attempt schedule. Strict configuration requires an explicit non-empty secret whenever a webhook URL is configured; the only controlled destination is the exact Compose receiver.

**Consequences:** The complete end-to-end lifecycle (upload → extract → human review → approve exact version → CSV / webhook export) is durable, idempotent, and audited. Raw secrets, full URLs, query/userinfo, internal network details, and unapproved versions are not persisted or exposed.

## ADR-013 — Stage 6 static application delivery and presentation boundary

**Status:** Accepted (Stage 6)

**Context:** Through Stage 5 the Compose demo published only the JSON API and health routes. The React interface existed exclusively behind the Vite development proxy, so `docker compose up` produced a running backend with no reachable product. That contradicts the Stage 6 goal that the landing page leads into a real working `/app`, blocks the ADR-005 requirement to capture factual media from the real application, and makes the release criterion "clone-to-demo instructions work" untrue. Serving browser assets is a new trust boundary: it introduces path resolution, response headers, and a client execution context that the JSON API did not have.

**Decision:** The API optionally serves one pre-built static bundle. The directory is server configuration (`WEB_DIR`); when it is empty or absent the API registers no static routes and behaves exactly as it did through Stage 5, so a clean `go build` and the existing tests never depend on a Node build.

When configured, the whole bundle is read into memory once at startup and never touched on the filesystem again. Requests resolve through an in-memory map keyed by the exact cleaned request path, so no request string ever reaches a filesystem call and path traversal, symlink escape, and directory listing are structurally impossible rather than filtered. Loading is bounded: a maximum file count and total byte budget, and only an allowlist of asset extensions is accepted. Content types are derived from that allowlist, never from file content or client input.

Route precedence is explicit. `/api/`, `/healthz`, and `/readyz` are matched before any static pattern; an unmatched `/api/` path returns the existing JSON error envelope and never falls back to HTML. The SPA fallback serves `index.html` only for `GET`/`HEAD` requests that are not under those prefixes and do not carry a known asset extension; unknown asset paths return 404 rather than HTML.

Every static response carries `X-Content-Type-Options: nosniff` and a fixed restrictive `Content-Security-Policy` with `default-src 'self'`, `base-uri 'none'`, `frame-ancestors 'none'`, `form-action 'self'`, and no `unsafe-inline` or `unsafe-eval` for scripts. Hashed build assets are immutable and cached; `index.html` is `no-store` so a redeployed bundle cannot be served from a stale shell. The document source stream keeps its existing `no-store`/`nosniff`/inline contract unchanged.

The landing page describes only shipped behavior. It contains no metric, customer, accuracy figure, or compliance statement, and its product imagery originates from real fictional-fixture runs of this application.

**Consequences:** `docker compose up` now yields a complete demonstrable product on one loopback port, and real product media can be captured from it. This is asset delivery only: it adds no authentication, no session, no user-supplied content rendering, and no multi-user authorization claim. Production deployments would still place their own TLS termination, cache, and access control in front of this boundary.

**Amendment (Stage 6 review).** The project code and security reviewers audited this boundary and found no high-severity issue; traversal, symlink escape, directory listing, content-type spoofing, and route confusion for real endpoints are structurally prevented. Two refinements were applied from that review. First, the fallback is registered on the bare `/` pattern (all methods) instead of `GET /`, so the reserved-prefix guard runs for every method: an unmatched or method-mismatched `/api/`, `/healthz`, or `/readyz` request now returns the JSON `route_not_found` envelope for any method (previously a non-GET request received Go's bare `405`), while method-specific API routes still win by specificity and a non-GET request to a non-reserved client path is a hardened `405`. The reserved-prefix check is case-insensitive. Second, the CSP tightens `object-src` to `'none'`, the 405 path emits the full hardened header set, and the load byte-budget is enforced against the bytes actually read. New tests in `cmd/api` and `internal/webui` cover the all-method envelope and the header/limit changes.

## ADR-014 — Optional live OpenAI structured-extraction provider

**Status:** Accepted

**Context:** Through Stage 7 the only structured extractor was the deterministic
`FakeStructuredExtractor`, which keeps the no-key demo runnable but never
exercises a real model. The `StructuredExtractor` port (ADR-010) was always
designed so a live provider could be added behind it without changing the
server's validation, normalization, evidence, or audit guarantees. Adding one
is user-directed and must not weaken any of those boundaries or the no-key
default.

**Decision:** Add `OpenAIStructuredExtractor`, an opt-in adapter behind the
existing `StructuredExtractor` interface. It is selected only when
`EXTRACTOR=openai`; the default remains `fake`, so a clean build and the whole
test suite never require a key or a network call. `EXTRACTOR=openai` requires a
non-empty `OPENAI_API_KEY`; the model (`OPENAI_MODEL`, default `gpt-4o-mini`)
and base URL (`OPENAI_BASE_URL`, default `https://api.openai.com/v1`) are server
configuration. The key lives only in process configuration, is sent solely in
the `Authorization` header, and is never logged, returned in an error, or placed
in a proposal.

The adapter reuses every existing trust boundary rather than adding a new one.
It calls Chat Completions with `temperature: 0` and a strict `json_schema`
response format that mirrors exactly the fields `DecodeProposalJSON` accepts, so
the reply decodes through the same unknown-field-rejecting strict decoder and
the same `Limits` byte/line/evidence bounds. The document reference text is sent
as clearly delimited untrusted data with a system instruction to ignore any
instruction inside it. Input is already bounded by `ValidateStructuredInput`;
the response read is bounded to `MaxProviderOutputBytes` plus a small envelope
allowance, and the parsed content is re-bounded by `ValidateProposal`. The
adapter never asserts evidence — it cannot prove a source excerpt — so it drops
any model-supplied evidence and lets the worker's normalizer, evidence check,
and diagnostic sanitizer run unchanged. A non-200 status, refusal, empty
content, oversized body, or schema violation is a bounded extraction error that
flows into the existing retry/dead-letter path.

**Consequences:** The full pipeline can now run against a real model without any
change to server authority: the model still cannot control identity, storage,
status, approval, export, or secrets, and every value it returns is validated
and normalized server-side. The default demo is unchanged and remains offline.
Outbound network access in the default extraction path is still disabled
(ADR-006); it is enabled only for this explicitly opted-in provider. Rate-limit
and auth failures are treated as generic extraction errors rather than being
finely classified; that refinement is deferred.

**Amendment (Stage 8).** The offline default is now a chain rather than a single
adapter: the fixture registry answers first, and `HeuristicStructuredExtractor`
runs only when it recognized nothing. See ADR-015. The live provider is
deliberately not chained.

**Amendment (live verification).** A first live run against `gpt-4o-mini`
surfaced three defects, all outside the adapter itself, now fixed.

1. The persisted job summary is generic by design, so the actual provider
   failure was invisible to an operator. `Worker.OnProviderError` is a narrow
   optional hook that forwards only `ErrOpenAIRequest`/`ErrOpenAIConfiguration`
   — the two errors whose messages are bounded and secret-free by construction.
   `cmd/worker` logs them with the document id. Tool output, storage paths,
   document text, and the API key never reach it.
2. The runtime image installed no `ca-certificates`, so every outbound TLS
   handshake failed. Nothing before this needed public TLS (the fake extractor
   is offline, the Compose receiver is plain HTTP), so the gap was invisible.
   The Dockerfile now installs `ca-certificates=20230311+deb12u1`, pinned like
   the other runtime packages.
3. An adapter that asserts no evidence leaves a nil slice, which marshalled to
   JSON `null` and violated the `invoice_versions_extraction_snapshot_shape`
   check constraint. The worker now encodes evidence, diagnostics, and warnings
   through a non-nil slice, so any adapter is safe.

With those fixed, a live run reaches `needs_review` with correct supplier,
invoice number, dates, subtotal, tax, total, and per-line quantities and unit
prices. The committed fixtures print no currency anywhere, so a live model
correctly returns `currency: null` and the server emits
`missing_or_invalid_currency` warnings; that is the designed
never-invent-a-value behavior, not an extraction fault. The keyed offline fake
supplies a currency, so the no-key demo is unaffected.

## ADR-015 — Offline heuristic fallback extraction and layout-preserving PDF text

**Status:** Accepted (Stage 8)

**Context:** Through Stage 7 the offline default was `FakeStructuredExtractor`
alone. It matches a document by committed SHA-256 plus an embedded marker, so it
recognizes exactly the four fictional fixtures in `testdata/` and nothing else.
Any other document — including the first real invoice a visitor tries — reached
`needs_review` with every field empty, no warnings, and a single
`fake_fixture_unmatched` diagnostic. The behavior was documented honestly, but a
portfolio demo that answers an ordinary upload with a blank form reads as broken
rather than as careful. Making a paid provider the default was rejected: the
no-key, offline, deterministic demo is a stated project invariant.

A second, quieter problem surfaced while validating this. Poppler's `pdftotext`
was invoked without `-layout`, so it emitted column-major text: on a normal
invoice every label ("Subtotal") landed on a different line from its amount.
That degrades the reference text handed to *any* extractor, a model included.

**Decision:** The offline default becomes a two-adapter chain behind the
unchanged `StructuredExtractor` port.

`FallbackStructuredExtractor` runs the fixture registry first and consults the
fallback only when the primary returned no candidate values at all. The trigger
is `Proposal.HasCandidates()` — the shape of the result — not a provider-specific
diagnostic string, so the chain stays provider neutral. A primary *error* is
returned unchanged and never masked, because a bounded, classified extraction
failure must keep flowing into the existing retry and dead-letter path. Both
adapters' diagnostics are preserved, bounded by `MaxDiagnostics`, so a reviewer
can still see that the registry was consulted and did not match. The live OpenAI
provider is deliberately *not* chained: a model returning nothing is a provider
result an operator should inspect, not a case for a regexp second opinion.

`HeuristicStructuredExtractor` is a deterministic, offline, stateless regex
reader. It is a demo affordance, not an accuracy claim, and it is constrained so
that its failure mode is silence rather than fabrication:

- A pattern that does not match leaves the candidate nil. Nothing is defaulted
  to zero, and no value is inferred from another.
- Only unambiguous date formats are read: ISO `YYYY-MM-DD`, `DD.MM.YYYY`, and
  English month names. Slash dates are skipped outright, because `03/04/2026`
  cannot be resolved without guessing a locale and ADR-007 forbids that.
- A rate is never an amount: a number followed by `%` is skipped, so "VAT (19%)
  13.49" yields 13.49, and a row carrying only a rate yields no candidate.
- Per-line tax is left unknown rather than assumed zero, so the normalizer's
  line-arithmetic check stays silent instead of raising a mismatch this reader
  cannot substantiate.
- Supplier name — by far the weakest signal, since it is just "the first
  plausible heading" — is proposed only when another field corroborated that the
  document is an invoice at all.
- A summary row is not a line item, and a tax registration number is not a tax
  amount.

Its evidence is truthful by construction: every entry quotes the exact source
line the value was read from, so `ValidateEvidence`'s substring check passes on
real data. This is the first adapter that can honestly assert evidence, and it
does so without any change to the server's verification.

`pdftotext` gains the fixed literal `-layout` argument, under the same output
cap, timeout, and argument-array invocation as before.

**Consequences:** An unseen invoice now produces real, evidence-backed
candidates, and the review screen demonstrates provenance instead of emptiness.
Nothing about server authority changes: the heuristic result is re-validated by
`ValidateProposal`, re-checked by `ValidateEvidence`, normalized by `money-v1`,
and warned on exactly like a model response, and its diagnostic code is
allowlisted and rewritten server-side rather than trusted. Every committed
fixture keeps its byte-identical snapshot, because the registry still answers
first. The heuristic will mis-read unusual layouts; that is accepted and stated,
which is precisely why its output is a proposal a human must approve. This ADR
adds no network access, no dependency, and no new executable.

ADR-006's recorded invocation is amended by the `-layout` flag.

## ADR-016 — Public demo deployment boundary

**Status:** Accepted (Stage 8). Configuration and hardening are implemented; no
public instance is operated by this repository.

**Context:** A portfolio reader is far more likely to click a link than to
install Docker, so a reachable demo is worth real engineering. But every earlier
decision in this log assumed a loopback instance with one server-configured
actor: ADR-003 states plainly that the demo makes no multi-user authorization
claim. Putting that same process on the public internet does not weaken any
server-side invariant, yet it does change who can reach it — which makes it a
boundary worth stating rather than an ops detail.

**Decision:** Public exposure is opt-in server configuration, and it grants
nothing. `PUBLIC_DEMO=true` changes exactly one thing: the interface renders a
notice saying the instance is shared, has no sign-in, is periodically erased,
and must receive only fictional documents. The flag is served from
`GET /api/v1/config`, whose payload is a single boolean — an unauthenticated
route must never carry a secret, a destination, or anything that grants
authority. The browser is told, not trusted: no client value influences any
server decision.

`UPLOAD_RATE_PER_MINUTE` bounds uploads per client address, checked before the
request body is read, so a refused caller cannot make the server consume 20 MiB
per attempt. The client is identified by transport peer address only. A
forwarded-for header is deliberately not honoured: any caller can set one, so
trusting it would let a single client mint a fresh allowance per request. A
deployment behind a proxy must therefore enforce its own limit at that proxy —
this limiter protects one process from casual abuse and makes no distributed
claim. Both settings default to off, so the local demo is byte-for-byte
unchanged.

What a public instance must still have, and what this repository does not
provide, is stated rather than implied: there is no authentication, no
authorization, no per-visitor isolation, and no production CSRF defence. Anyone
who reaches the instance can read and act on every document in it. That is
acceptable for a demo whose only content is fictional and disposable, and
unacceptable for anything else.

Operating an instance is therefore a deliberate act by the repository owner,
documented in `docs/DEPLOYMENT.md`: ephemeral database, no persistent volume,
fictional fixtures only, the offline extractor (no provider key), and a
scheduled reset. This ADR does not deploy anything and no public URL is claimed
anywhere in this repository.

**Consequences:** The demo can be published without contradicting ADR-003,
because publishing changes reachability rather than authority. The honest
limitation — a shared, unauthenticated, disposable workspace — is stated in the
product itself, not only in the README, so a visitor cannot mistake it for a
multi-tenant application. Adding real authentication remains the separate P0
backlog item it always was.

**Amendment (Stage 9) — the deployment shape is now committed, and the webhook
receiver stays.** The guidance above left the platform open. In practice one
constraint decides it: there is no S3 adapter, so `STORAGE_DIR` is a filesystem
that the API and the worker must both open, and most PaaS offerings attach a
disk to exactly one service. Satisfying them would mean a supervisor process
starting both binaries in one container — new code, and a new failure mode where
nothing restarts a dead worker. A single host running the committed Compose
files needs none of that, so that is the documented shape:
`docker-compose.public.yml` plus `deploy/` (nginx, reset script, systemd timer).

Three points of that override are decisions rather than mechanics.

First, the **per-visitor rate limit moves to the proxy**. This ADR has always
said a deployment behind a proxy must limit at that proxy; the override makes
the consequence explicit rather than implied. Because `X-Forwarded-For` is not
trusted, every request behind the proxy reaches the process with one peer
address, so `UPLOAD_RATE_PER_MINUTE` bounds the *instance*, not the visitor. It
is kept as a backstop and set accordingly; `deploy/nginx/invoiceflow.conf`
carries the real limit, with a separate and much stricter zone for
`POST /api/v1/documents` — the only expensive, state-creating route.

Second, the **controlled webhook receiver is retained**, which reverses this
document's earlier instruction to leave `WEBHOOK_URL` unset on a public
instance. That instruction was aimed at pointing a public demo at a *real*
external destination. The Compose receiver is not one: it is the exact fixed
internal destination of ADR-012, unreachable from outside the Compose network,
and it verifies signature, timestamp window, and idempotency key before
accepting a request. Leaving it unconfigured would make the signed-webhook
export — retries, dead-lettering, audit — invisible to every visitor, which
removes a large part of what the project demonstrates. The real hazard was never
the destination but the secret: `docker-compose.yml` ships
`local-demo-webhook-secret`, which is committed and therefore public, so the
override *requires* an explicit `WEBHOOK_SECRET` and refuses to start without
one.

Third, **ephemerality comes from the scheduled reset**, not from the absence of
a volume. The earlier text called for container-local storage, which cannot work
when two containers must share the directory. Postgres runs with no volume at
all; `document_data` remains a named volume and is destroyed nightly by
`deploy/reset.sh`. The override also pins `EXTRACTOR=fake` and blanks the OpenAI
variables, because the base file reads them from the host environment and an
inherited key must neither opt the instance in nor be handed to a container that
has no use for it.

None of this deploys anything. No public instance is operated and no URL is
claimed anywhere in this repository.

## ADR-017 — Request tracing and an operational metrics endpoint

**Status:** Accepted (Stage 9)

**Context:** Every JSON error envelope has always carried a `request_id`, but
`writeAPIError` generated it at the moment of writing and nothing logged it. The
affordance existed and the trace did not: a user could report an id that no
search would ever find. The API had no access logging at all — `slog` was used
only for process start and stop.

Separately, `AGENTS.md` lists "Health, readiness, structured logs, and basic
metrics" in the MVP scope. The first three existed; metrics did not. That was
the only place where the project failed its own declaration, and the gap was
closed by a line in the README's limitations table rather than by code.

**Decision:** One middleware assigns a request id **at the edge**, before any
handler runs. It publishes the id in `X-Request-Id`, puts it in the request
context, and emits exactly one structured line per request:
`method / path / status / duration_ms / request_id`. `writeAPIError` reads the
id from the context instead of minting its own, so the value a client quotes is
the value in the header and the value in the log.

Only server-derived or sanitized values are logged. The path is client
controlled, so it is truncated and reduced to printable ASCII, and the query
string is dropped entirely; request bodies, uploaded filenames, storage keys,
document text, and secrets never reach the log (the `AGENTS.md` sanitization
rule). When the middleware is absent — a handler constructed directly in a test
— `requestID` falls back to a fresh id so an envelope is never issued without
one.

Metrics are a small dependency-free registry (`internal/metrics`) rendering the
Prometheus text format. Counters and gauges carry at most one label dimension,
and every label value is a server-owned constant (a job outcome) or a database
status, so cardinality is bounded by code rather than by data. The instruments
are: `invoiceflow_process_jobs_total` and `invoiceflow_export_jobs_total` by
terminal outcome (`success`, `retry`, `dead_letter`),
`invoiceflow_extraction_duration_seconds` as a histogram whose bounds bracket
the configured PDF (15 s) and OCR (30 s) timeouts, and two gauges,
`invoiceflow_documents` and `invoiceflow_jobs`, by status. Queue depth is the
`ready` series of the latter.

The counters are reported by optional `Worker` hooks that fire **only after the
durable transition committed**, so a counter never claims an outcome the
database did not record. To make `dead_letter` exact rather than inferred,
`FinishRetry` and `FinishExportRetry` now return the outcome they committed;
their behaviour is otherwise unchanged. The two gauges are collected at scrape
time from `SELECT status, count(*)`, not counted in memory, so a restarted
worker reports the truth immediately instead of counting up from zero. A gauge
whose query fails is rendered as a comment and suppresses only itself; one
database hiccup must not blind every metric, and the driver's message never
reaches the exposition.

The endpoint is served by the **worker** process — the process that owns the
jobs and the database view — on its own listener, `METRICS_ADDR`, which is
**empty by default**, so no listener is opened at all unless an operator asks
for one. It has no authentication of its own and reveals traffic volume and
queue depth, so configuration refuses an address equal to `API_ADDR`: that is
the one mistake that would silently publish operational volume on the public
instance of ADR-016. A deployment must bind it to an address that is not
publicly reachable.

**Consequences:** A reported `request_id` is now findable, and the project meets
the observability line in its own MVP scope. Nothing about domain authority,
the database schema, or the public data contract changes: the middleware writes
one header and one log line, and the metrics endpoint is read-only, aggregate,
and off by default. This is not a full observability stack — there is no
tracing, no exemplars, no per-route latency histogram, and no alerting — and
none of those is claimed.
