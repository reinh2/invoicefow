# Current Task

## State

Stage 0 is complete. This repository is an **uninitialized InvoiceFlow agent pack**, not a partially implemented application. It contains instruction, planning, template, and agent-configuration files only. There is no Git worktree, Go module, React/Vite project, database schema or migrations, Docker/Compose configuration, test suite, runnable service, fixture, or media asset.

The architecture and visual direction below are proposed Stage 0 decisions, not implemented behavior. Stage 1 must not begin until the recommended architecture and visual direction are approved.

## Inspection record

- Read: `MASTER_PROMPT.md`, `MASTER_PROMPT_PREMIUM_DESIGN.md`, `AGENTS.md`, `CLAUDE.md`, every file in `docs/`, and the relevant `.codex/`, `.claude/`, and `.kiro/` configuration, agent, command, prompt, and steering files.
- Repository inventory: only the agent pack exists; no application source or package/tool manifests are present.
- Tooling found: Go 1.26.5, Node 26.3.0, npm 11.16.0, Docker 29.2.1, Docker Compose 5.0.2, GNU Make, Git, and FFmpeg. `psql`, `pdftotext`, `pdfinfo`, `qpdf`, and `tesseract` are not on the host path. Docker daemon access was not verified.
- Validation run: `python3 scripts/check-agent-pack.py` passed.
- Git status and history cannot be inspected because this checkout is not a Git worktree. The root has `gitignore.fragment`, but no active `.gitignore`; `.DS_Store` is present.
- Independent read-only reviews completed for backend architecture, PostgreSQL/durable jobs, security, document AI, frontend/accessibility/performance, and visual/motion/media design. No agent changed files.

## Recommended architecture

### Backend and operational topology

Use the documented modular monolith target:

```text
React/Vite browser
  -> Go API (`cmd/api`)
       -> PostgreSQL via pgx
       -> server-owned object-storage port (filesystem implementation for demo)
  -> Go worker (`cmd/worker`)
       -> PostgreSQL durable processing/export jobs
       -> bounded PDF-text, OCR, and structured-extractor ports
```

Create the following package boundaries without microservices:

```text
cmd/api/                 HTTP API, health/readiness, dependency wiring
cmd/worker/              durable job consumer, dependency wiring
internal/app/            configuration and application composition
internal/documents/      intake, storage metadata, duplicate policy
internal/invoices/       canonical invoice values, money, normalization, warnings
internal/processing/     job lifecycle, leases, retries, orchestration
internal/extraction/     text/OCR/provider ports, fake provider, evidence
internal/review/         immutable review versions, approval and rejection
internal/exports/        CSV and signed webhook adapters
internal/audit/          append-only audit model and queries
internal/platform/       PostgreSQL, migrations, storage, process runner, logging
db/migrations/           forward SQL migrations
web/                     strict React/TypeScript/Vite application
```

Use app-generated UUIDs; exact monetary values are `BIGINT` minor units plus an explicit ISO currency. All timestamps are UTC/RFC 3339 at the API boundary. Browser, uploaded bytes, extracted/OCR text, model output, webhook targets/responses, and external tool output remain untrusted.

### Durable data and transaction design

Proposed durable entities: `stored_objects`, `documents`, `jobs`, `job_attempts`, immutable extraction/review version snapshots, versioned line items/warnings/evidence, approval/rejection records, export records/attempts, and append-only `audit_events` with a per-document sequence.

Required atomic boundaries are:

1. Final durable-storage promotion, then one database transaction for document metadata, upload audit, and processing-job enqueue. Storage is outside PostgreSQL, so failures require a safe orphan-reconciliation policy.
2. Job claim, lease-token assignment, and attempt start.
3. Extraction snapshot, warnings/evidence, document transition to `needs_review`, and audit event.
4. Immutable review snapshot and edit audit event.
5. Compare-and-swap approval or rejection of one exact review version and its audit event.
6. Exact approved-version export-job enqueue and audit event.
7. Export attempt/result plus the terminal document transition and audit event.

Jobs are at-least-once: claim ready rows with a short `FOR UPDATE SKIP LOCKED` transaction, lease them with a token, recover expired leases transactionally, and require the lease token to heartbeat or complete. OCR, provider, and webhook calls happen outside transactions. External retries retain one idempotency key; webhook delivery is honestly at-least-once with downstream deduplication, not exactly-once.

### Processing and AI boundary

```text
stored original -> bounded server-owned work file
  -> PDF text extraction or OCR fallback
  -> bounded page-labelled reference text
  -> strict structured candidate DTO
  -> server-only normalization and warning generation
  -> immutable proposal/review version -> needs_review
```

Keep three independent ports: `TextExtractor`, `OCR`, and `StructuredExtractor`. The worker selects image-to-OCR; it tries PDF text extraction first and falls back to OCR only when documented text-sufficiency rules fail. The default extractor is a deterministic, offline fake provider keyed by a server-known fictional fixture marker/content hash; unknown input follows a deterministic partial-review or failure path.

Provider DTOs contain proposal fields and optional evidence only. Strict decoding rejects unknown, duplicate, trailing, malformed, oversized, and wrongly typed JSON. Dates and decimal strings are normalized server-side; canonical data never uses floating point. Server-generated warning codes distinguish malformed provider output from a safely persisted partial proposal. Provider output cannot set identity, storage keys, state, approval, export destination, retries, secrets, or confidence-based authority.

### Frontend architecture

Use one strict TypeScript React/Vite project with browser-native `fetch` and local component state initially. Do not add routing, state-management, PDF-rendering, or animation libraries until a concrete need and bundle cost are established.

```text
web/src/
  app/          route shells and route-level failure/loading boundaries
  landing/      marketing sections and story composition
  upload/       accessible file-input/drop-zone states
  documents/    list/detail status views
  review/       source viewer, form, warnings, approval/export controls
  components/   focused reusable product primitives
  api/          typed clients, DTO mapping, stable error-envelope parsing
  domain/       UI-safe state and money/date presentation helpers
  styles/       CSS-variable tokens, reset, primitives, responsive layout
  motion/       reduced-motion-aware reveal/state-transition utilities
  media/        live-DOM story and later video/poster fallback component
  test/         test setup and API mocks
```

Routes are `/` for the landing page and `/app` for the working demo. Server DTOs are mapped once in `api/`; the UI never calculates financial truth, invents permission, or fabricates a processing/export state. Async state is explicit (`idle`, `loading`, `success`, `empty`, `duplicate`, `failure`, `retrying`).

Desktop review uses a stable two-pane source/review grid at 1024px and above. Tablet uses an accessible source/review selector; mobile uses one column and preserves labelled form context. Field provenance is textual and programmatic as well as visual: Extracted, Warning, Edited, and Approved.

## Recommended visual direction

### Options considered

| Direction | Character | Trade-off |
| --- | --- | --- |
| **Calibrated Ledger — recommended** | Near-black/navy operational surfaces, warm paper source documents, cool blue processing/focus, amber warnings, green approval; thin “evidence rails” connect source, proposal, correction, and export. | Best expression of trust and human review; requires careful contrast testing between dark UI and paper panel. |
| Editorial Control Room | Refined light stone palette, strong editorial type, subtle cobalt accent. | Highly legible but less cinematic; state differentiation is less distinctive. |
| Midnight Blueprint | Blue-black grid, cyan details, technical diagrams. | Supports the architecture story but is at risk of cyberpunk styling and competing with invoice content. |

**Recommendation: Calibrated Ledger.** It makes the original invoice physically and conceptually distinct from the extracted proposal, while remaining calm, expensive, technically precise, and non-generic. It also avoids unsupported AI-automation claims.

### Design-token proposal

- Surfaces: base `#0C1018`, raised `#131A25`, cool-slate hairlines, warm document paper `#F5F0E8`.
- Semantic states: electric blue processing/focus, amber warning, green approved/exported, red destructive/error. Every state also has a label/icon; color is never the only cue.
- Typography: local/system-first sans stack with tabular numerals; remote fonts are deferred.
- Rhythm: 4px/8px spacing scale; explicit container, radius, shadow, breakpoint, focus-ring, and z-index tokens.
- Motion: `120ms`, `200ms`, `320ms`, `520ms`; standard, emphasized, and decelerate easing tokens. Routine motion uses `transform` and `opacity` only.

## Motion, scroll story, and media plan

Stage 1 uses CSS transitions/keyframes plus Intersection Observer. A small shared motion layer supplies reveal, status transition, and panel-shift primitives. It animates only truthful server/UI states: upload acceptance, queued/processing, warning reveal, saved edit, approval, export, and audit appearance. It must not create fake progress, confidence, metrics, or outcomes.

The landing-page hero/story uses a **live-DOM scroll-controlled interface transformation**, not a primary video or image sequence:

```text
raw fictional invoice -> extraction -> structured fields
  -> server warnings -> human correction/approval -> export/audit
```

On desktop/tablet, a normal-flow section has a contained `position: sticky` visual frame and six discrete phases. Scroll work is `requestAnimationFrame`-throttled and writes a CSS custom property rather than causing per-scroll React renders. It never traps scrolling. On mobile, coarse pointers, and `prefers-reduced-motion`, it becomes ordinary static sequential content with the full narrative in HTML.

No footage is created in Stages 1–4. After the fictional end-to-end demo exists, Stage 6 may record a 12–20 second real demo and ship an optimized muted `playsInline` WebM with MP4 fallback, a poster, reserved dimensions, lazy loading, a static fallback, and documented regeneration steps. No stock footage, commercial assets, fake logos/testimonials, or invented metrics are permitted.

## Accessibility and performance safeguards

- Semantic landmarks, one `h1` per route, skip link, visible `:focus-visible`, logical tab order, labelled native inputs, and no hover/drag/motion-only action.
- Drop zone is a keyboard-operable file input. Warnings use validation summaries, `aria-describedby`, and linked controls. Dialogs manage focus, Escape safely, and restore focus.
- Restrained live regions announce real upload, processing failure, save, approval, and export changes—not decorative motion.
- Global `prefers-reduced-motion` disables scroll scrubbing, parallax, and continuous decoration; state changes remain understandable without animation.
- Initial landing JavaScript target: <=175 KB gzip; investigate any increase before exceeding 250 KB. Initial CSS target: <=30 KB gzip. Lazy-load `/app` and heavy preview code.
- Noncritical desktop media target: <=2 MB total; mobile media <=1 MB; poster <=250 KB. Image sequences are disallowed unless justified and kept <=1.5 MB.
- Reserve media geometry; target CLS <=0.1, LCP <=2.5s, and INP <=200ms in a representative throttled production check.

## Threat-control-test matrix

| Threat | Owner/control | Required proof |
| --- | --- | --- |
| Malicious/oversized upload | API intake limits; extension, declared MIME, signature, dimensions/page limits; streaming SHA-256 | Rejection tests for size, mismatch, polyglot/truncated/encrypted files and hash races |
| Traversal, symlink, command injection | Server-generated keys; private work dirs; no-follow/exclusive storage; fixed argv with contexts | Hostile filename/path/symlink, fixed-argv, timeout, and output-cap tests |
| PDF/OCR resource exhaustion | Bounded process runner, fixed tools/container, time/output/page/pixel limits | Malformed, decompression-bomb-like, timeout, and excessive-output tests |
| Prompt injection/model overreach | Bounded page-labelled data; strict proposal DTO; server-only state authority | Adversarial text/JSON tests for unknown fields, status/export injection, and malformed output |
| Unauthorized review/approval/export | Local-demo authority is server-configured and loopback-bound by default; later auth is server-side | Unauthorized/cross-principal tests when multi-user mode is introduced; spoofed actor must not alter audit actor |
| Webhook SSRF/replay/duplicate effects | Server-managed HTTPS destination, private-address denial on each connection, no redirects, HMAC-SHA-256, timestamp/delivery/idempotency key | Private/IPv6/DNS-rebinding/redirect/timeout/signature/replay/idempotency tests |
| Storage/DB split failure | Promotion-before-enqueue protocol plus orphan reconciliation | Fault injection before/after storage promotion and DB commit |
| Lost/duplicate job work | Transactional claim/lease token/recovery/backoff/dead letter | Concurrent claim, stale lease, heartbeat, retry, and recovery tests |
| Audit mutation or partial transitions | Append-only DB enforcement; state and audit in one transaction | Update/delete rejection, injected audit failure, transition and pagination tests |
| Secret/document leaks | Active `.gitignore`, placeholder-only `.env.example`, redacted structured logs, fictional fixtures | Sensitive-path/fixture policy and log-redaction checks |

## Complete staged implementation plan

| Stage | Scope and exit criteria |
| --- | --- |
| **0 — complete** | Inventory, independent reviews, architecture/design/security plan, risk register, and this living-document update. No application code was created. |
| **1 — foundation and design system** | Initialize Git if authorized; active ignore policy; Go module with API/worker entry points; config, pgx, forward migration runner with advisory lock and ledger; Compose PostgreSQL; initial domain/job/audit constraints; health/readiness; storage/extraction ports and offline fake-provider skeleton; strict React/Vite `/` and `/app` shells; Calibrated Ledger tokens/primitives/motion/reduced-motion foundation; baseline Go, PostgreSQL, frontend, accessibility, build, and CI checks. |
| **2 — secure upload and durable processing** | Multipart/signature/hash validation; filesystem storage with promotion/orphan policy; deterministic duplicate response; atomic document/audit/job creation; transactional claim, lease, heartbeat, retry/dead-letter/recovery; secure intake and concurrency tests; real upload and processing-state UI. |
| **3 — extraction and validation** | Pinned PDF/OCR adapters behind process ports; deterministic fake extraction; strict decode, normalization, exact arithmetic, warnings, evidence; immutable extraction persistence and `needs_review`; hostile-input/retry tests; landing processing narrative component. |
| **4 — review and approval** | Document/review/audit query APIs; responsive source-plus-form review; immutable human edits and line items; exact-version compare-and-swap approve/reject; confirmation and dirty-state UX; accessibility and transition tests. |
| **5 — export and integration** | Exact approved-version CSV; server-managed signed webhook; idempotency, retry, dead letter, replay/SSRF safeguards; export/audit UI; integration and crash-recovery tests. |
| **6 — premium landing and scroll story** | Complete marketing story with real product UI, live-DOM scroll sequence, responsive/reduced-motion fallback, optional factual demo media, media-size checks, and Lighthouse/accessibility checks. |
| **7 — portfolio release** | Three fictional fixtures, no-key Compose smoke from upload through export, screenshots and honest README, regenerated architecture/API docs, demo script, and final code/database/security/frontend/accessibility/performance reviews. |

## Stage 1 task

### Goal

Create only the independently testable project foundation and shared visual system. Do not accept real uploads, implement extraction, claim that the application works, or create final video assets.

### Expected files

```text
.gitignore                         (activate the reviewed ignore policy)
.env.example                       (placeholders only)
go.mod, cmd/api/, cmd/worker/, internal/app/, internal/platform/
internal/documents/, internal/processing/, internal/extraction/, internal/audit/
db/migrations/, docker-compose.yml, Dockerfile(s), Makefile, CI configuration
web/package.json, web/tsconfig*.json, web/vite.config.ts, web/src/
docs/ARCHITECTURE.md, docs/API_CONTRACT.md, docs/DECISIONS.md
```

### Acceptance criteria

- A fresh checkout can start PostgreSQL and run forward migrations safely with a migration advisory lock.
- API and worker have separate runnable entry points; `/healthz` reports liveness and `/readyz` reports real database readiness.
- Initial schema enforces document states, exact money representation, immutable/auditable foundations, and durable-job metadata without pretending upload/extraction is complete.
- `/` and `/app` exist with an accessible, responsive Calibrated Ledger token layer, static honest product frame, route-level failure states, focused product primitives, and reduced-motion support.
- The frontend exposes no invented processing data, metrics, or backend capability.
- Unit/integration/frontend checks cover the foundation: state rules, migration/readiness, route/focus/upload-control accessibility, and reduced-motion behavior.
- The default configuration uses a fixed server-side local-demo actor and binds public API exposure to loopback by default; multi-user authentication is explicitly deferred and never implied.

### Planned validation surface

Stage 1 must introduce and run the following project-native commands before it is accepted:

```text
make fmt
make lint
make test
make test-integration
make frontend-test
make build
make up
make check
```

`make check` must aggregate formatting, static checks, unit tests, PostgreSQL integration, frontend type/build/test/accessibility checks, and the agent-pack check. A full upload-to-export smoke command belongs to Stage 7, not Stage 1.

## Open decisions and risks

1. **Approval required:** accept Calibrated Ledger and the live-DOM scroll story as the implementation direction before Stage 1 begins.
2. **Git initialization:** this is not currently a Git worktree. Initializing it is needed for normal stage traceability, but requires explicit authorization.
3. **Demo authority:** proposed local-only fixed server identity must be implemented before approval/export endpoints exist; a production/multi-user authorization model is out of scope until explicitly designed.
4. **Review model:** recommended initial policy is immutable version snapshots and a `409` for edits after approval/export. Amendments should create a new workflow, not mutate an approved record.
5. **PDF/OCR implementation:** choose pinned tools and licensing/container strategy before Stage 3. Host binaries are unavailable, so adapters must be containerized and optional at Stage 1.
6. **Extraction policy:** OCR sufficiency threshold, supported currencies/exponents, rounding-policy version, max byte/page/pixel/prompt/output sizes, and evidence display constraints require ADRs before Stage 3.
7. **Webhook policy:** default demo has no outbound webhook destination. Before Stage 5, define server-managed destination configuration, secret storage, HMAC canonicalization, timestamp/replay window, and SSRF/DNS-rebinding defenses.
8. **Draft API:** response schemas, pagination, source-file authorization, mutation payloads, and stable error-code taxonomy must be made concrete in the API contract as each stage implements them.

## Exact next prompt

> Approve the Stage 0 Calibrated Ledger direction and live-DOM scroll story, and authorize Git initialization. Then implement Stage 1 only according to `docs/CURRENT_TASK.md`: foundation, durable database/migration baseline, accessible React/Vite shells, design/motion tokens, and tests—without upload, extraction, approval, export, or final media.
