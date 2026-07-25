# Current Task

## Remove hero motion and synchronize pipeline stages

The hero clip was removed from both the page and the delivered media bundle at
the repository owner's request. The original static review-preview returns
because it supports the opening message without adding scroll height or
competing for attention.

The pipeline clip no longer maps progress to the height of its wrapper. Instead,
`PipelineScene` maps 0–1 from the vertical centre of the first stage card to the
centre of the sixth card, and passes that calculation to `ScrubVideo`. The video
therefore advances only as the reader reaches the next explanatory stage. The
single-column desktop layout remains deliberately spacious, and the 0.1 smoothing
factor keeps small wheel and trackpad motions fluid without visibly lagging behind
the active card.

## Refreshed factual product capture

`web/public/media/demo.webm` and its `demo-landing-poster.png` were regenerated
from an isolated, freshly seeded Compose demo after the hero removal and the
pipeline synchronization change. The recording now shows the static opening,
the current scroll scenes, and the vertically paced six-stage pipeline rather
than an earlier landing-page revision. `demo-review.png` was regenerated from
the same fixture session so reduced-motion and no-video visitors receive a
matching real-application fallback.

## Earlier scroll-scene finding — hero purpose and pipeline pacing

Visual review found that the initial hero left the supplied art's intentional
right-side negative space unexplained, while the pipeline retained its original
two-column card grid. The latter made the whole 61-frame clip traverse only
three rows of copy, so the visual moved too quickly to read as six related
stages.

That intermediate hero legend was subsequently removed at the repository
owner's request. The final pipeline treatment keeps the single, generously
spaced desktop sequence, but its owner-provided stage-centre mapping (described
above) replaces the earlier wrapper-height mapping and tuning experiment. The
existing narrow viewport and reduced-motion behavior remain unchanged.

## Supplied hero and pipeline clips — scroll-scrubbed installation

The supplied clips were initially installed into the browser bundle. The hero
clip was later removed; the retained `Pipeline.mp4` master is available as
`web/public/media/pipeline.mp4`, re-encoded to 1280×720, 12 fps, no-audio,
all-intra MP4 (61 frames). This is essential for an arbitrary scroll seek not to
decode from the start of the clip. It remains ordinary static media, so the
hardened bundle handler retains byte-range support for seeking.

The retained clip uses the existing `ScrubVideo` scroll-playhead mechanism and
sticks opposite the six server stages. At widths below 901px the sticky treatment
is removed by the shared component and the clip resolves to its final state
instead of running an arbitrary autoplay animation. It has no separate poster
image: when reduced motion is requested, `ScrubVideo` seeks a paused video
element to its final frame; existing callers with posters continue to render a
still image.

The existing `ScrubVideo` test suite now also covers that no-poster,
reduced-motion path, and the landing-page tests assert the retained pipeline
source while confirming the hero source is absent. The source changes passed the
targeted Vitest suite and desktop browser review.

## Provenance story — scroll-scrubbed scene (ADR-018)

The provenance walkthrough is now a two-column scene: a scroll-scrubbed clip that
sticks on the left while the four step cards scroll past it on the right. The
clip is generated art — a sheet of paper, a second sheet layering in behind it,
and three plotter-drawn lines ending in blue, amber and green markers — and
ADR-018 records why generated media is admissible at all: it is
non-representational, contains no interface, no legible text and no product
claim, and the ADR-005 obligation is still discharged by the captured
`demo.webm`, not by this.

`web/src/components/ScrubVideo.tsx` drives the playhead from scroll position and
nothing else: a paused element, progress from `getBoundingClientRect()` in a
`requestAnimationFrame` loop, easing toward the target, seeks only past a frame of
drift, and the loop gated by an `IntersectionObserver` so an off-screen section
never drives a decoder. Reduced motion drops all of it and renders the clip's
final frame as a still — the existing `DemoMedia` contract. Eleven tests cover
it, including the cases that are easy to get wrong: the element is never
autoplaying or controllable, the playhead clamps at both ends, an unknown duration
leaves it alone, an unlaid-out wrapper stays at the start, and the reduced-motion
path constructs no observer at all.

The scrub wrapper takes its height from the grid row — from the cards — rather
than from a viewport multiple, so the playhead spans exactly the four states.
Measured at 1440x900: a quarter of the scene is 2.52 s of a 10.08 s clip, so each
of the clip's four staged beats lands opposite its own card. Below 901px the panel
is a plain block above the cards, and there a wrapper shorter than the viewport
resolves to the clip's **last** frame rather than its first, because a frozen
first frame shows the setup of the animation and none of its point.

**A real defect in the Go bundle handler was found by this.** `internal/webui`
answered every request with `200` and the whole body — no `Accept-Ranges`, no
`206`. For scripts, styles, and images that is fine; for a video it is fatal,
because a browser seeking past what it has buffered asks for a byte range, and a
server that ignores the ask makes seeking impossible rather than slow. The clip
therefore showed its poster and never moved when served from the built bundle,
while working on the Vite dev server, which does support ranges. `write` now
goes through `http.ServeContent`, and a test asserts `206`, the exact bytes, and
that the hardened header set still applies to a partial response. The
pre-existing `demo.webm` could not be seeked either; nobody noticed because it
plays linearly.

**The hero application of the same component was reverted** before this. It was
built and measured, then removed: giving the copy room forced the clip full-bleed
and cropped to the viewport, which enlarged the paper by 125% and turned composed
art into wallpaper. The static hero preview is restored unchanged. Two findings
from that attempt are why the story clip works: the art must be generated with an
**even background** (the hero clip's upper-left corner is a lit mid grey, not the
page ink, so any letterboxed placement showed a hard band), and it must be
**re-encoded all-intra** — both delivered masters carried roughly one keyframe per
file, and without `-g 1` every seek decodes from the start.

The clip is **square**, not 16:9. The motion in it is horizontal — document, then
lines, then markers — so a vertical frame has nowhere to put that path, while
16:9 in a 530px column left the portrait sheet 297px tall with half the frame
empty above and below it. At 1:1 the panel is 529x529 and the sheet is twice as
tall. The asset is `web/public/media/story-scrub.mp4` (1024x1024, 12 fps, 121
frames, all keyframes, no audio, 1.6 MB) with `story-scrub-poster.jpg` (41 KB) as
poster and reduced-motion still. Frame rate is traded for size deliberately: with a
scroll-driven playhead the reader perceives position, not frame rate. The
generation constraints and the exact ffmpeg invocation are in ADR-018.

**Validation:** `format:check`, `lint`, `typecheck`, 51 frontend tests, and
`vite build` pass. In a real browser at 1440x900 and 375x812: the wrapper height
matches the card column exactly, the playhead tracks scroll linearly across five
sampled positions, the stacked layout puts the panel above the cards with no
sticky positioning, and there is no horizontal overflow. **Not verified in a
browser:** the reduced-motion path (unit tests only) and the Compose-served
bundle.

## Stage 9 — distribution, traceability, and documentation accuracy

Every Stage 9 item except the public deployment is implemented. The deployment
itself needs a hosting account and credentials and publishes an instance to the
internet, so it remains the repository owner's action, not a code change.

### P1 — request tracing (ADR-017)

`writeAPIError` generated a `request_id` at the moment of writing and nothing
logged it: the affordance existed and the trace did not. `withRequestContext`
(`cmd/api/middleware.go`) now assigns the id **at the edge**, publishes it in
`X-Request-Id`, puts it in the request context, and emits exactly one
structured access line per request — `method`, `path`, `status`, `duration_ms`,
`request_id`. `writeAPIError` reads that id instead of minting its own, so the
value a client quotes is the value in the header and the value in the log; a
test asserts the two are equal, and another asserts one log record per request.

The path is client controlled, so it is truncated to 200 bytes and reduced to
printable ASCII with the query string dropped. Request bodies, uploaded
filenames, storage keys, document text, and secrets never reach the log. When
the middleware is absent — a handler built directly in a test — `requestID`
falls back to a fresh id, so an envelope is never issued without one.

### P1 — basic metrics (ADR-017)

`AGENTS.md` lists "Health, readiness, structured logs, and basic metrics" in the
MVP scope; the first three existed and the fourth did not. `internal/metrics` is
a small dependency-free registry rendering the Prometheus text format:
`invoiceflow_process_jobs_total` and `invoiceflow_export_jobs_total` by terminal
outcome, `invoiceflow_extraction_duration_seconds` (bounds bracketing the 15 s
PDF and 30 s OCR timeouts), and `invoiceflow_documents` / `invoiceflow_jobs` by
status, with queue depth as the latter's `ready` series.

Two design points matter more than the instrument list. The counters fire from
optional `Worker` hooks **only after the durable transition committed**, so a
counter never claims an outcome the database did not record; to make
`dead_letter` exact rather than inferred, `FinishRetry` and `FinishExportRetry`
now return the outcome they committed. And the two gauges are read at scrape
time from `SELECT status, count(*)` rather than counted in memory, so a
restarted worker reports the truth immediately instead of counting up from zero.
A gauge whose query fails renders as a comment and suppresses only itself — one
database hiccup must not blind every metric — and the driver's message never
reaches the exposition.

The endpoint belongs to the **worker**, on its own listener `METRICS_ADDR`,
empty by default so nothing is opened unless asked for. It has no authentication
and reveals traffic volume, so configuration refuses an address equal to
`API_ADDR`: that is the one mistake that would silently publish operational
volume on the ADR-016 public instance.

### P1 — documentation matched to the code

The discrepancies all understated the project. `README.md`'s limitations table
claimed "No document list/search" when the keyset-paginated list shipped in
Stage 8, and its status line still said stages 0–7. `docs/CURRENT_TASK.md`
listed `document list/search` as unimplemented for the same reason. Each now
names only what is genuinely absent — search, manual retry — and the metrics
line is gone because metrics now exist.

### P2 — coverage, hygiene, and an end-to-end check

**`internal/processing` unit coverage** rose from 4.1% to 10.6% by extracting
the decisions that never needed a database — `clampDocumentPageSize`,
`requireNeedsReview`, `requireExportableVersion`, and the two export
idempotency-key builders — and testing them directly alongside the cursor
round-trip, `validUUID`, diagnostic sanitization, and error classification. The
transactional paths stay with the integration suite, which is where they belong.

**`web/node_modules` left the Go module.** `go test ./...` used to descend into
`web/node_modules/flatted/golang/pkg/flatted`, a third-party Go package inside
an npm dependency. `go.mod` now carries `ignore ./web/node_modules`.

**The `act()` warning is gone.** `routes.test.tsx`'s file-input test rendered the
shell without mocking `fetch` and without awaiting, so the mount requests
settled after the test body.

**`ReviewWorkspace` became a reducer.** Sixteen `useState` hooks are now one
state machine in `web/src/components/review/reviewState.ts`. Two facts drove
eight of those booleans — which irreversible action is being confirmed, and
which one is in flight — and both are single-valued by construction, so
representing them as one value each removes states that were unreachable but
still had to be reasoned about. Eight reducer tests cover the transitions,
including the ones that are easy to get wrong: a failed action leaves its
confirmation dialog open, a background refresh failure does not blank out the
review, and a manual refresh restarts the bounded poll counter without dropping
the watched export.

**Browser end-to-end tests.** `web/e2e/review-flow.spec.ts` drives upload →
review → correction → approval → CSV download in Chromium against a running
demo, plus a second test that a document is reachable from the list rather than
only from the URL the upload returned. It uploads a generated fictional invoice
rather than a committed fixture, for two reasons: uploads are deduplicated by
SHA-256, so a fixture would `409` on a second run, and an unseen document
exercises the ADR-015 heuristic reader — the path a real visitor takes. The
suite is deliberately outside `make check` because it needs a running instance;
`make test-e2e` documents how to point it at an isolated one.

### The deployment: prepared, not performed

No public instance is operated and no URL is claimed anywhere in this
repository. What changed is that everything short of the act itself is now
committed rather than described.

The platform choice is forced by one constraint: there is no S3 adapter, so
`STORAGE_DIR` is a filesystem the API and the worker must both open, and most
PaaS offerings attach a disk to exactly one service. Satisfying them would need
a supervisor starting both binaries in one container — new code, and a new
failure mode where nothing restarts a dead worker. One host running the existing
Compose files needs none of that, and the base file was already written for it
(shared `document_data`, API published to loopback only).

- `docker-compose.public.yml` — turns on `PUBLIC_DEMO`, sets the upload
  backstop, drops the Postgres volume and every published port except the API's,
  pins `EXTRACTOR=fake` and blanks the OpenAI variables (the base file reads them
  from the host environment, so an inherited key must not opt the instance in),
  and refuses to start without an explicit `WEBHOOK_SECRET` — the committed
  `local-demo-webhook-secret` is public by definition.
- `deploy/nginx/invoiceflow.conf` — TLS and the **real** per-visitor rate limit.
  This is the part that is easy to get wrong: `X-Forwarded-For` is deliberately
  not trusted, so behind a proxy every request reaches the process with one peer
  address and `UPLOAD_RATE_PER_MINUTE` bounds the instance rather than the
  visitor. A separate, much stricter zone covers `POST /api/v1/documents`.
- `deploy/reset.sh` plus a systemd service and timer — nightly wipe, which is
  what makes a shared unauthenticated workspace acceptable.

Two documentation defects were fixed while doing this. `docs/DEPLOYMENT.md`
called for "container-local storage for `STORAGE_DIR` — no mounted volume",
which cannot work when two containers must share that directory; ephemerality
comes from the scheduled reset instead. And its instruction to leave
`WEBHOOK_URL` unset is reversed for the internal receiver, with the reasoning
recorded as an amendment to ADR-016: that receiver is not a real external
destination but the exact fixed internal one of ADR-012, and dropping it would
hide the signed-webhook export, retries, and dead-lettering from every visitor.
The real hazard was the committed secret, which the override now refuses to use.

The checklist gained steps for verifying `X-Request-Id`, confirming the metrics
address is unreachable from outside, and — the one failure that would turn a
demo into a place strangers store files — confirming the reset actually erases.

**Not validated:** the nginx configuration has not been checked against a
running nginx, and the Compose override was verified only with
`docker compose config` (the Docker daemon was unavailable). The step-by-step
guide says so and puts `nginx -t` before enabling the site.

### Validation

`gofmt`, `go vet ./...` (including `-tags=integration`), `go test ./...`, and the
frontend gate (`format:check`, `lint`, `typecheck`, 42 tests, `vite build`) all
pass. **Not re-run in this pass:** the PostgreSQL integration suite, the Compose
smoke, and the new browser end-to-end suite — the Docker daemon was unavailable
on this machine. The integration test that calls `FinishRetry` was updated for
its new return value and compiles under `go vet -tags=integration`, but it has
not been executed against a database.

## Stage 8 — complete except the deploy itself

Every P0, P1, and P2 item of the Stage 8 demo-experience pass is implemented.
The one thing left is operating a public instance, which needs a hosting account
and credentials and is the repository owner's action, not a code change.

### P2 — quality and public-demo readiness

**Frontend tooling and structure.** Prettier and ESLint (typescript-eslint
type-checked rules plus react-hooks) now run in `make frontend-test` and in CI
before the build. The whole frontend is reformatted, and `ReviewWorkspace.tsx`
— 288 lines with individual lines up to 2142 characters — is split into
`components/review/{ReviewForm,ReviewContext,ConfirmDialog,SourcePanel,ReviewMessage}.tsx`,
leaving the workspace as a state orchestrator. Lint rules were chosen to catch
defects rather than style, and they earned that: `react-hooks/set-state-in-effect`
found three real cascading renders. All three were fixed properly — in
`ProvenanceStory` the "reveal everything" fallback became derived state instead
of a write-back from an effect, and in `ReviewWorkspace` and `DocumentList` the
error reset moved into the success handler, which also stops the message
flickering during a reload.

**Public-demo readiness (ADR-016).** `PUBLIC_DEMO=true` renders a notice stating
the workspace is shared, has no sign-in, is periodically erased, and must
receive only fictional documents. The flag is served from `GET /api/v1/config`,
whose payload is a single boolean because the route is unauthenticated.
`UPLOAD_RATE_PER_MINUTE` bounds uploads per client address and is checked
*before* the request body is read, so a refused caller cannot make the server
consume 20 MiB per attempt. `X-Forwarded-For` is deliberately ignored: any
caller can set it, so honouring it would hand out a fresh allowance per request
— a deployment behind a proxy must limit at that proxy, which ADR-016 and
`docs/DEPLOYMENT.md` both state. Both settings default to off, so the local demo
is unchanged. Verified live in Compose: `{"public_demo":true}`, the notice in
the served bundle, and a 2/minute limit producing `201, 201, 429, 429` with
`Retry-After: 60`.

**Not done: the deployment.** No public instance is operated and no URL is
claimed anywhere in this repository. `docs/DEPLOYMENT.md` carries the
configuration, the ephemeral-data requirements, and a post-deploy checklist.

### P1 — self-consistency with the project's own rules

**Missing-field warnings.** `AGENTS.md` requires warning on missing fields, but
the normalizer only ever warned about values that were present and rejected, so
an empty proposal reached the reviewer with no warnings at all.
`missingFieldWarnings` now reports `missing_required_field` for `supplier_name`,
`invoice_number`, `issue_date`, `currency`, and `total`. It inspects the raw
candidate rather than the normalized result, so a value that was supplied and
rejected keeps its specific warning (`invalid_date`, `invalid_money`,
`unsupported_currency`) instead of being counted twice. Subtotal and tax are
deliberately not required: many legitimate invoices state only a total, and
warning on those would train a reviewer to ignore warnings.

**Field-level warning presentation.** Warnings carry the exact field they
concern, down to `line_items.0.total`, but the review form ignored it and showed
everything in one list at the bottom. Each input now renders its own warnings
with `aria-invalid`, `aria-describedby`, and an amber marker from the existing
token set; the summary list remains. The warning list is rendered *outside* the
`<label>` — nesting it folded the warning text into the input's accessible name
instead of leaving it a description, which an existing test caught.

**Document list.** A document used to be reachable only through the URL returned
at upload, so closing the tab lost it. `GET /api/v1/documents` returns a bounded
page, newest first, with keyset pagination on `(created_at, id)` rather than
offset: a document inserted mid-paging cannot make a row repeat or disappear.
The page size is clamped server-side (20 default, 100 maximum) and the cursor is
opaque and validated — a foreign cursor is `400 invalid_pagination`, never a
silent reset to page one, which would loop a paging client forever. The
projection is presentation-safe (no SHA-256, object id, storage key, or internal
version UUID) and an integration test asserts that. `/app` renders the table
with exact money formatted from integer minor units, never float division.

### P0 — demo experience

**Offline heuristic fallback (ADR-015).** The offline default is now a chain:
`FallbackStructuredExtractor` asks the fixture registry first and falls through
to `HeuristicStructuredExtractor` only when the primary returned no candidate at
all. Before this, any document outside the four committed fixtures reached
`needs_review` with an entirely empty form — the single worst first impression
in the project. The heuristic is deterministic, offline, stateless, and
constrained so that its failure mode is silence rather than fabrication: no
value is defaulted or inferred, locale-ambiguous slash dates are skipped, a
percentage is never read as an amount, per-line tax stays unknown rather than
zero, and a supplier name requires corroboration from another field. Its
evidence quotes the exact source line, so `ValidateEvidence` verifies it against
real page text. Server authority is unchanged: the result passes the same
`ValidateProposal`, `ValidateEvidence`, `money-v1` normalization, and warning
generation as a model response, and its diagnostic code is allowlisted and
rewritten server-side in `sanitizeDiagnostics`.

Two defects were found by running this against the live Compose demo rather than
only in tests. `pdftotext` was invoked without `-layout`, so Poppler emitted
column-major text that separated every label from its amount — fixed, and it
improves the reference text for any extractor including a model. And "VAT (19%)"
was read as a tax amount of 19.00 — a fabricated value, now excluded.
Verification on an unseen invoice: supplier, email, invoice number, both dates,
currency, subtotal, tax, total, and both line items all correct, with
`71.00 + 13.49 = 84.49` producing no spurious warning. Every committed fixture
keeps a byte-identical snapshot and `scripts/compose-smoke.sh` passes.

**Repository presentation.** The coding-agent harness (`MASTER_PROMPT.md`,
`MASTER_PROMPT_PREMIUM_DESIGN.md`, `MODEL_ROUTING.md`, `MANIFEST.json`,
`START_HERE_RU.md`, `gitignore.fragment`, `.kiro/`, `.codex/`) moved to
`.internal/`, which is git-ignored; the files remain on disk.
`check-agent-pack.py` moved with it and `agent-pack` left `make check`, since it
validates the harness rather than the product. `AGENTS.md`, `CLAUDE.md`, and
`.claude/agents/` were kept deliberately — they are standard agent-configuration
files, and moving `CLAUDE.md` would break project instruction loading. Git
history was **not** rewritten; the files are still present in earlier commits.

## State

Stage 6 — honest landing page, product story, and static application delivery — is **complete**. All five items are implemented and covered by tests, and the two follow-on items that blocked it (realistic fixtures, factual media) are done. The governing decision is ADR-013.

| # | Stage 6 item | State |
| --- | --- | --- |
| 1 | Serve one pre-built static bundle from the API behind `WEB_DIR` | Done |
| 2 | Replace the stale foundation-stage landing copy with shipped behavior | Done |
| 3 | Scroll-driven provenance scene with equivalent reduced-motion path | Done |
| 4 | Factual media asset captured from the running demo (ADR-005) | Done |
| 5 | Reformat and split the CSS layer by surface | Done |

What is implemented:

- `internal/webui` loads one bundle into memory at startup under file-count, total-byte, and extension-allowlist bounds, and answers every request from an exact map lookup, so no request string reaches a filesystem call. `WEB_DIR` empty or absent keeps the process API-only, exactly as through Stage 5.
- The bundle handler is registered last on the bare `/` pattern (all methods). Each API and health pattern is strictly more specific and wins by specificity, so `serveWebBundle` runs for every unmatched path regardless of method. `/api/`, `/healthz`, and `/readyz` are reserved and return the JSON envelope with code `route_not_found` rather than HTML **for any method** (the earlier `GET /` registration left a non-GET request to a mistyped API path answered by a bare 405); the reserved check is case-insensitive. Missing non-HTML assets return 404 instead of the shell. A non-`GET`/`HEAD` request to a non-reserved client path is a hardened 405 that carries the same CSP and `Referrer-Policy` as every other static response.
- Every static response carries `nosniff`, `Referrer-Policy: same-origin`, and a fixed first-party CSP with no `unsafe-inline`/`unsafe-eval` and `object-src 'none'`. Hashed assets are immutable-cached; the shell is `no-store`.
- The Dockerfile builds `web/` in a pinned Node stage and Compose sets `WEB_DIR=/app/web`, so `docker compose up` now serves the real product on one loopback port.
- `/` describes only shipped behavior, with the fictional `OFFICE-001` values the offline extractor is actually configured with, real server warning codes, and an explicit "what this deliberately does not do" section. There is no metric, customer, accuracy, or compliance claim.
- The provenance story renders every state as readable content at all times; `IntersectionObserver` only moves visual emphasis, so reduced-motion and no-observer paths present the same information.
- The CSS layer is split into `tokens`, `reset`, `shell`, `landing`, `upload`, `review`, `motion` and reformatted; `layout.css` and the unused `.empty-workspace` rule are gone.
- A "See it running" section (`web/src/components/DemoMedia.tsx`) embeds a muted WebM screen capture with a poster, and shows a static review-screen still instead of the video when the visitor prefers reduced motion — an equivalent no-video path.

Realistic fictional fixtures (also required by Stage 7) now back the demo. `testdata/` holds three visually realistic, fully invented documents generated by `scripts/gen-fixtures.py`: `fixture-aurora-stationery.pdf` (clean text PDF), `fixture-meridian-supplies.png` (image through the Tesseract OCR path), and `fixture-cedarline-services.pdf` (text PDF whose subtotal + tax ≠ total, producing exactly one `subtotal_tax_total_mismatch` warning). `cmd/worker` registers each with its committed SHA-256 and marker; `cmd/worker/main_test.go` reverifies the file↔registry hashes and normalizes each proposal through the real normalizer to confirm its warning set. The pinned Tesseract 5.3.0 reads the image marker verbatim, and the Compose smoke drives the OCR path end to end (asserting the Meridian snapshot) as well as the text-PDF path. The synthetic `OFFICE-001` file is retained only because the landing illustrates it by name.

The media asset (ADR-005) is captured from the real running application on those fixtures. `web/public/media/` holds `demo.webm` (muted landing scroll, ~2.3 MB), `demo-landing-poster.png` (poster/hero), and `demo-review.png` (the split review screen showing the seeded Meridian invoice beside its editable fields). Regeneration is a committed tool: `web/scripts/capture-media.mjs` (a `@playwright/test` devDependency, run via `npm --prefix web run capture:media`) drives an isolated seeded Compose demo. The regeneration command, formats, and byte budget are documented in that script's header.

The ADR-013 static-delivery boundary was reviewed by the project `code-reviewer` and `security-reviewer`. No high-severity finding: traversal/symlink-escape/directory-listing are structurally impossible, content-type integrity and route confusion for real endpoints are sound. The medium finding (non-GET reserved routes answered a bare 405 instead of the documented JSON envelope) and the low findings (405 lacking hardened headers; byte-budget accounting vs `stat.Size()`; case-sensitive reserved-prefix guard; `object-src 'self'`) are all fixed and covered by new tests.

Stage 5 — explicit approval and export (CSV and signed webhook) is `PASS`. The remediation is validated with unit, PostgreSQL integration, frontend, and isolated Compose checks. The forward-only schema changes are `0010_stage5_remediation.sql`, `0011_stage5_export_document_invariant.sql`, `0012_stage5_export_approved_version_invariant.sql`, and `0013_stage5_terminal_job_schedule.sql`; no earlier migration is rewritten.

## Implemented through Stage 5

- `POST /approve` requires an explicit current immutable version and `confirm: true`; approval stores an immutable reference and audit version in one transaction.
- CSV v1 is generated from the exact approved normalized snapshot using UTF-8, RFC 4180 quoting, CRLF records, and integer-minor-unit exact decimal formatting. Repeated requests are byte-identical and do not add audit events.
- Webhook export is a PostgreSQL durable `export_document` job. It stores only `server:webhook:v1` and a safe presentation label. The worker resolves the real URL and secret from server configuration, never from the export row or request.
- Processing and export lease recovery have separate lifecycles. Expired export leases close the attempt, schedule retry or dead-letter the export record, append a safe versioned audit event, and leave the approved document state unchanged.
- `exports.attempts` is atomically synchronized to the claimed durable job attempt on retry, success, permanent failure, lease recovery, and exhaustion. Terminal jobs and export records clear `next_attempt_at`; the API/UI render the durable export value rather than recalculating it from a job join.
- Two composite foreign keys require every export version to belong to the document and equal its immutable `approved_version_id`; direct insertion of another version of the same document is rejected.
- Strict webhook delivery is HTTPS-only, redirect-free, limited to port 443, rejects userinfo/query/fragment and private/reserved addresses, validates all DNS answers, uses a bounded timeout/body limit, and signs canonical JSON bytes with HMAC-SHA256.
- The Compose demo uses an explicitly configured controlled receiver at a fixed internal destination. It validates canonical payload, signature, timestamp window, and idempotency key before accepting a request.
- Strict webhook configuration has no fallback secret: a configured URL requires an explicit non-empty `WEBHOOK_SECRET`. The public export projection contains only safe destination fields and `version_number`; export-history database failures return an error rather than a partial 200 response.
- The review UI confirms approval and both export types, validates runtime export records, observes one enqueued webhook through bounded polling plus an accessible manual refresh, shows pending/retrying/succeeded/failed/dead-letter and refresh errors, and keeps modal focus keyboard-accessible with reduced-motion behavior. Export panel styles use shared CSS tokens and responsive split-review breakpoints.

## Deliberately not implemented

There is no invoice payment, bank connectivity, user-configurable webhook destination, production authentication, document search, manual retry endpoint, or raster OCR for scanned PDFs. A bounded, keyset-paginated document *list* was added in Stage 8 (`GET /api/v1/documents`); only search is absent.

## Optional live extractor (ADR-014)

The default structured extractor is still the deterministic offline `fake`
provider, so the no-key demo, a clean build, and the whole test suite need no
key or network. Setting `EXTRACTOR=openai` with a non-empty `OPENAI_API_KEY`
opts in to `internal/extraction/openai.go`, a live OpenAI adapter behind the
existing `StructuredExtractor` port. It requests strict `json_schema` output,
reuses the same strict decoder, `Limits`, evidence check, normalizer, and
diagnostic sanitizer as the fake path, sends the document text as delimited
untrusted data, asserts no evidence, and never logs or returns the key. Model
(`OPENAI_MODEL`, default `gpt-4o-mini`) and base URL (`OPENAI_BASE_URL`) are
server configuration. Covered by `internal/extraction/openai_test.go`.

This path was verified against the real API on 24 July 2026: a fictional text
PDF uploaded to the Compose demo with `EXTRACTOR=openai` reached `needs_review`
with supplier, invoice number, both dates, subtotal, tax, total, and every line
item's quantity and unit price extracted correctly. Three defects found by that
run are fixed and recorded in the ADR-014 amendment: the operator could not see
why extraction failed (`Worker.OnProviderError`), the runtime image had no
`ca-certificates` so all outbound TLS failed, and a nil evidence slice encoded
as JSON `null` and violated the snapshot shape constraint. The fixtures print no
currency, so a live model returns `currency: null` and the server warns — the
intended never-invent behavior. The offline `fake` default is unchanged.

## Validation target

The release checklist for this task is recorded in `docs/DEFINITION_OF_DONE.md` and `stage-5-review.md`. The Compose smoke uses an isolated project/volume and must not truncate the default persistent database.

Stage 6 was validated with `make fmt lint test`, the PostgreSQL integration suite (`make test-integration` against a throwaway database), `make frontend-test` (typecheck, 29 frontend tests, and the Vite build that bundles the media), and `sh scripts/compose-smoke.sh` on an isolated project/ports. The media was captured against a separate isolated Compose demo on `127.0.0.1:18081` seeded by `scripts/demo-seed.sh`, and the served landing page (video, poster, reduced-motion still) was confirmed in a real browser.

## Stage 7 release result

Stage 7 is complete. The isolated smoke now checks duplicate `409`, the
Cedarline warning and immutable correction, Meridian image OCR, and a controlled
webhook retry/dead-letter path before rejecting Aurora. Migration 0014 makes
export idempotency and export-job pairing database invariants; money aggregation
does not wrap `int64`. `docs/DEMO_SCRIPT.md` provides the 75-second walkthrough.

The final Go, PostgreSQL integration, Node frontend, clean Compose, and isolated
smoke gates passed on 24 July 2026. This remains a loopback fixed-actor demo:
authentication, authorization, and production CSRF are not claimed.
