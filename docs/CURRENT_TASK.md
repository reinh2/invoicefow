# Current Task

## State

Stage 6 — honest landing page, product story, and static application delivery — is **partially complete**. Items 1–3 and 5 below are implemented and covered by tests; item 4 is **not done** and blocks the stage. The governing decision is ADR-013.

| # | Stage 6 item | State |
| --- | --- | --- |
| 1 | Serve one pre-built static bundle from the API behind `WEB_DIR` | Done |
| 2 | Replace the stale foundation-stage landing copy with shipped behavior | Done |
| 3 | Scroll-driven provenance scene with equivalent reduced-motion path | Done |
| 4 | Factual media asset captured from the running demo (ADR-005) | **Not done** |
| 5 | Reformat and split the CSS layer by surface | Done |

What is implemented:

- `internal/webui` loads one bundle into memory at startup under file-count, total-byte, and extension-allowlist bounds, and answers every request from an exact map lookup, so no request string reaches a filesystem call. `WEB_DIR` empty or absent keeps the process API-only, exactly as through Stage 5.
- The bundle handler is registered last and only as `GET /`. `/api/`, `/healthz`, and `/readyz` are reserved and return the JSON envelope with code `route_not_found` rather than HTML. Missing non-HTML assets return 404 instead of the shell. Non-`GET`/`HEAD` returns 405. Both health routes are now method-scoped so this fallback is unambiguous.
- Every static response carries `nosniff`, `Referrer-Policy: same-origin`, and a fixed first-party CSP with no `unsafe-inline` or `unsafe-eval`. Hashed assets are immutable-cached; the shell is `no-store`.
- The Dockerfile builds `web/` in a pinned Node stage and Compose sets `WEB_DIR=/app/web`, so `docker compose up` now serves the real product on one loopback port.
- `/` describes only shipped behavior, with the fictional `OFFICE-001` values the offline extractor is actually configured with, real server warning codes, and an explicit "what this deliberately does not do" section. There is no metric, customer, accuracy, or compliance claim.
- The provenance story renders every state as readable content at all times; `IntersectionObserver` only moves visual emphasis, so reduced-motion and no-observer paths present the same information.
- The CSS layer is split into `tokens`, `reset`, `shell`, `landing`, `upload`, `review`, `motion` and reformatted; `layout.css` and the unused `.empty-workspace` rule are gone.

Out of scope for Stage 6: authentication, document list/search, manual retry, raster OCR for scanned PDFs, live paid providers, and any metric, customer, accuracy, or compliance claim.

## Blocking Stage 6 completion

1. **No media asset exists yet (ADR-005).** The real UI was verified in a real browser against a Compose demo seeded by `scripts/demo-seed.sh` — loopback navigation now works, so this is no longer an environment limitation. Writing an optimized image or video file into the repository still needs a headless-capture toolchain (for example Playwright or a headless-Chrome container) that this repository does not depend on. Adding that dependency is an open decision.
2. **The fictional PDF fixtures are not visually realistic.** `testdata/stage2-fictional-compose.pdf` is a 605-byte synthetic file, so the review screen's source panel renders as an essentially empty page. Any product screenshot taken today shows that. Stage 7 already requires at least three realistic fictional fixtures; the media asset should be captured after that work, not before.

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

There is no invoice payment, bank connectivity, live AI provider, user-configurable webhook destination, production authentication, document list/search, manual retry endpoint, raster OCR for scanned PDFs, or landing media story.

## Validation target

The release checklist for this task is recorded in `docs/DEFINITION_OF_DONE.md` and `stage-5-review.md`. The Compose smoke uses an isolated project/volume and must not truncate the default persistent database.

The earlier note that loopback navigation is blocked no longer holds. Stage 6 drove a real browser against an isolated Compose demo on `127.0.0.1:18081` and confirmed the served landing page, the scroll-driven emphasis, the split review screen with real seeded data, and the mobile layout. What remains manual is producing an optimized media file, not viewing the application.

## Exact next prompt

> Create realistic fictional invoice fixtures, then capture and commit the Stage 6 media asset from the running demo.
