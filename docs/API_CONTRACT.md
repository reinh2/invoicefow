# API Contract

## Implemented operational endpoints

| Method and path | Behavior | Response |
| --- | --- | --- |
| `GET /healthz` | Liveness probe; it does not contact PostgreSQL. | `200 {"status":"ok"}` |
| `GET /readyz` | Readiness probe; performs a bounded PostgreSQL ping. | `200 {"status":"ready"}` or `503 {"status":"not_ready"}` |

Every API response carries `X-Request-Id`, an opaque id assigned at the edge before any handler runs (ADR-017). The same value appears in the `request_id` field of every error envelope and in exactly one structured access log line per request (`method`, sanitized `path`, `status`, `duration_ms`, `request_id`). The header is informational: no request value influences it, and quoting it back grants nothing.

The API binds to `127.0.0.1:8080` by default. Compose publishes it only to loopback; set `API_HOST_PORT` or `POSTGRES_HOST_PORT` when either local port is already in use.

## Implemented Stage 6 browser delivery

When `WEB_DIR` names a directory holding a built browser bundle, the API also serves that bundle (ADR-013). When it is empty or absent, no static route is registered and the process serves the JSON API only.

| Method and path | Behavior | Response |
| --- | --- | --- |
| `GET`/`HEAD` `/assets/{name}` | Exact lookup of one hashed build asset. | `200` with the allowlisted content type and `Cache-Control: public, max-age=31536000, immutable`, or `404` |
| `GET`/`HEAD` any other non-reserved path | Serves the application shell so the client router can resolve `/`, `/app`, and `/app/documents/{id}`. | `200 text/html` with `Cache-Control: no-store` |

Rules that hold for every static response:

- `/api/`, `/healthz`, and `/readyz` are reserved (matched case-insensitively). An unmatched path below them returns the normal JSON envelope with code `route_not_found`, never HTML, **for any HTTP method** — a non-GET request to a mistyped API path is not answered with a bare `405`.
- A request for a missing file that carries a non-HTML extension returns `404`; it is never answered with the shell.
- For a non-reserved path, only `GET` and `HEAD` are served; anything else returns `405` with an `Allow: GET, HEAD` header and the standard static security headers.
- Every response carries `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, and a fixed `Content-Security-Policy` with `default-src 'self'`, `object-src 'none'`, `base-uri 'none'`, `frame-ancestors 'none'`, and no `unsafe-inline` or `unsafe-eval`.
- Content types come from an extension allowlist. Files with any other extension are not loaded into the bundle and cannot be served.

## Implemented Stage 2 document intake

### `POST /api/v1/documents`

Accepts exactly one `multipart/form-data` part named `file`. `Content-Encoding` must be absent or `identity`; request bodies above 21 MiB and files above 20 MiB are rejected. The extension, parsed declared part media type, and leading signature must agree on one of:

| Extension | Declared and stored media type | Required validation |
| --- | --- | --- |
| `.pdf` | `application/pdf` | `%PDF-` header and `%%EOF` in its last KiB |
| `.jpg`, `.jpeg` | `image/jpeg` | JPEG SOI plus bounded full standard-library decode |
| `.png` | `image/png` | PNG signature plus bounded full standard-library decode |

JPEG and PNG dimensions are bounded to 10,000 pixels per dimension and 40,000,000 total pixels. This is not a full PDF parser: the worker revalidates encrypted and semantically malformed PDFs with the pinned Stage 3 toolchain before extraction.

On success, the server streams and hashes bytes, promotes them under a server-generated storage key, then atomically records the stored object, queued document, `document_uploaded` audit event, and `process_document` job.

**Success — `201 Created`**

```json
{"document":{"id":"opaque UUID","status":"queued","created_at":"2026-07-23T12:34:56Z"}}
```

The SHA-256 hash, original filename, storage key, and filesystem path are not returned.

**Errors** use this stable envelope. Every error contains an opaque request ID.

```json
{"error":{"code":"invalid_file","message":"file could not be accepted","request_id":"opaque UUID"}}
```

| Status | Implemented codes | Meaning |
| --- | --- | --- |
| `400` | `invalid_request`, `invalid_file` | Multipart shape, request encoding, metadata, signature, PDF end marker, or image decoding was invalid. |
| `409` | `duplicate_document` | The SHA-256 was already accepted; no existing document identity is disclosed. |
| `429` | `rate_limited` | Only when `UPLOAD_RATE_PER_MINUTE` is configured (off by default). Checked before the body is read, and carries `Retry-After` in seconds. The client is identified by transport peer address; `X-Forwarded-For` is deliberately ignored. |
| `413` | `file_too_large` | The request or file exceeded an intake limit. |
| `500` | `storage_error`, `internal_error` | Intake could not complete. |

This endpoint sends `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

## Internal Stage 3 processing

The worker claims queued process jobs, validates/extracts PDFs through the pinned toolchain, invokes the deterministic offline fake provider, persists an immutable extraction snapshot, and transitions a successful document to `needs_review`. It records generic dead-letter failures for encrypted/malformed PDFs and bounded retry failures for transient processing faults.

## Implemented Stage 4 review API

All Stage 4 document identifiers are UUIDs. The default local demo uses its configured server actor and makes no user authorization claim. Responses are bounded to the latest 100 immutable versions and 100 latest audit events. They never expose a browser filename, storage key, filesystem path, SHA-256, raw provider response, or secret.

### `GET /api/v1/documents/{document_id}`

Returns one document's read-only review representation. It includes the document's status and media type; immutable extraction and human-review versions ordered newest first; each version's raw proposal, normalized proposal, server warnings, evidence, sanitized diagnostics, `money-v1` policy name, and an exact-decimal `editable` form representation; plus latest audit events ordered newest first. `editable` uses strings rather than browser floating-point values.

**Success — `200 OK`**

```json
{"document":{"id":"opaque UUID","status":"needs_review","media_type":"application/pdf","versions":[{"version_number":1,"source":"extraction","rounding_policy_version":"money-v1","editable":{"currency":"USD","total":"24.00","line_items":[]},"warnings":[],"evidence":[],"diagnostics":[]}],"audit":[{"sequence":3,"action":"processing_completed","actor":"system","payload":{},"occurred_at":"2026-07-23T12:34:56Z"}]}}
```

### `GET /api/v1/documents/{document_id}/source`

Streams the server-owned original using its stored PDF/JPEG/PNG media type for the split review screen. It uses `Content-Disposition: inline`, `Cache-Control: no-store`, and `X-Content-Type-Options: nosniff`; no storage or source identity metadata is returned.

### `GET /api/v1/config`

Returns the presentation flags the browser needs. The route is unauthenticated,
so its payload is deliberately one boolean: it must never carry a secret, a
destination, a path, or anything that grants authority.

**Success — `200 OK`**

```json
{ "public_demo": false }
```

`public_demo` is `true` only when the server is configured with
`PUBLIC_DEMO=true` (ADR-016). It changes what the interface says, never what any
caller is permitted to do.

### `GET /api/v1/documents`

Returns one bounded page of documents, newest first. Read-only: it changes no
state.

| Query parameter | Meaning |
| --- | --- |
| `limit` | Optional page size. Must be a positive integer; the server clamps it to at most 100 and defaults to 20. |
| `cursor` | Optional opaque cursor from a previous response's `next_cursor`. Clients must pass back exactly what they received and must not construct one. |

Pagination is keyset, not offset: the cursor carries the `(created_at, id)` of
the last row returned, so a document inserted while a client pages cannot make a
row repeat or disappear. `next_cursor` is absent on the last page.

Each row is a presentation-safe summary of the document's newest version, which
is a proposal under review and not authoritative financial data. The SHA-256,
stored-object id, storage key, filesystem path, and internal version UUID are
never returned.

**Success — `200 OK`**

```json
{"documents":[{"id":"opaque UUID","status":"needs_review","created_at":"2026-07-23T12:34:56Z","updated_at":"2026-07-23T12:35:10Z","supplier_name":"Northwind Trading GmbH","invoice_number":"NW-2291","currency":"EUR","total_minor":8449,"version_number":1}],"next_cursor":"opaque cursor"}
```

`total_minor` is exact integer minor units under the `money-v1` policy and is
absent when the version has no total. `supplier_name`, `invoice_number`,
`currency`, and `version_number` are absent when unknown.

| Status | Implemented codes | Meaning |
| --- | --- | --- |
| `400` | `invalid_pagination` | `limit` was not a positive integer, or `cursor` was not one this server issued. |

### `POST /api/v1/documents/{document_id}/human-reviews`

Creates one new immutable `human_review` version only while the document remains `needs_review`. The request must identify the version the user edited and only accepts strict candidate fields. It accepts no state, actor, approval, export, evidence, diagnostics, storage, or authority fields.

```json
{"base_version":1,"proposal":{"supplier_name":"Fictional Vendor","supplier_email":"","invoice_number":"INV-1","issue_date":"2026-07-20","due_date":"","currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Service","quantity":"2","unit_price":"10.00","tax_amount":"0.00","total":"20.00"}]}}
```

The server applies the persisted `money-v1` policy and date/arithmetic normalization, generates the new snapshot's warnings, carries only source evidence and sanitized diagnostics forward, then atomically inserts the version and `human_review_saved` audit event. It never mutates an existing extraction or human-review version.

**Success — `201 Created`**

```json
{"version_number":2}
```

### `POST /api/v1/documents/{document_id}/reject`

Requires a confirmed JSON body and atomically transitions `needs_review` to terminal `rejected` with a `document_rejected` audit event. It does not delete the source or create/change an invoice version.

```json
{"confirm":true}
```

**Success — `204 No Content`**

## Implemented Stage 5 approval and export API

### `POST /api/v1/documents/{document_id}/approve`

Requires an explicit `version_number` and `confirm: true`. Atomically locks the document, verifies status is `needs_review`, associates the document with `approved_version_id`, transitions status to `approved`, and appends a `document_approved` audit event.

```json
{"version_number":2,"confirm":true}
```

**Success — `200 OK`**

```json
{"document":{"id":"opaque UUID","status":"approved","approved_version_number":2}}
```

### `GET /api/v1/documents/{document_id}/export/csv`

Requires the document to be in state `approved` or `exported`. It exports the exact immutable `approved_version_id`, never the browser's latest form state. Format `csv-v1` is UTF-8 without a BOM, RFC 4180 comma-separated with double-quote escaping, CRLF record endings, and this exact header/order:

`supplier_name,supplier_email,invoice_number,issue_date,due_date,currency,subtotal,tax_amount,total,line_item_description,line_item_quantity,line_item_unit_price,line_item_tax_amount,line_item_total`

Each line item is one record with invoice metadata repeated. An invoice without line items has one record with empty line-item columns. Money is rendered from exact integer minor units using the server currency exponent (`USD/EUR/GBP/RUB` two decimals, `JPY` zero); negative values keep a leading minus. No browser floating-point conversion is involved. Atomically updates status from `approved` to `exported` and appends a `csv_exported` audit event. Subsequent calls return identical bytes without duplicating audit events.

**Success — `200 OK`**

Headers:
- `Content-Type: text/csv; charset=utf-8`
- `Content-Disposition: attachment; filename="invoice-{document_id}-v{approved_version}.csv"`
- `X-InvoiceFlow-CSV-Format: csv-v1`
- `Cache-Control: no-store`
- `X-Content-Type-Options: nosniff`

### `POST /api/v1/documents/{document_id}/export/webhook`

Requires the document to be in state `approved` or `exported`. Enqueues a durable `export_document` webhook job in PostgreSQL for the server-configured destination and appends an `export_enqueued` audit event. The request cannot supply a URL, destination, secret, or delivery target. API responses and review detail expose only an opaque destination reference (`server:webhook:v1`) and safe label (`Server-configured webhook`).

The worker resolves the actual URL and secret only from server configuration. Strict mode is the default: HTTPS, port 443, no redirects, no userinfo/query/fragment, private/reserved IP ranges denied for every DNS answer, validated dial pinning, bounded 10-second request timeout and 256 KiB response limit. Compose uses an explicit exact controlled receiver adapter at its internal fixed address. Payload JSON is canonical and signed as `HMAC-SHA256(secret, timestamp + "." + body)`. The receiver must validate the RFC3339 timestamp within five minutes, signature with constant-time comparison, canonical body, and idempotency key. Retries keep the same idempotency key and body.

**Success — `202 Accepted`**

```json
{"export":{"id":"opaque UUID","document_id":"opaque UUID","version_number":2,"export_type":"webhook","status":"pending","idempotency_key":"webhook_export:{document_id}:{approved_version_uuid}","destination_ref":"server:webhook:v1","destination_label":"Server-configured webhook","attempts":0,"created_at":"2026-07-23T12:34:56Z","updated_at":"2026-07-23T12:34:56Z"}}
```

Export records use `pending`, `retrying`, `succeeded`, `failed`, or `dead_letter`; they expose the safe approved `version_number`, attempt count, safe error summary, and (when scheduled) the next attempt time. The record's `attempts` is the durable projection of the claimed job attempt and is atomically updated for retry, success, permanent failure, lease recovery, and retry exhaustion. Terminal export records have no `next_attempt_at`; their paired terminal jobs also clear the schedule. They do not expose the internal `invoice_versions.id`. The persisted `idempotency_key` is the same value returned here, placed in the canonical webhook body and sent as `X-InvoiceFlow-Idempotency-Key`; retry and lease recovery do not change it or the canonical body bytes. Lease recovery records the retry/dead-letter audit event and does not change the approved document to `failed`.

The database rejects any export whose version is not exactly `documents.approved_version_id`: one composite foreign key confirms that the version belongs to the document and a second binds `(exports.document_id, exports.version_id)` to the immutable approved reference. The API therefore never has a path that can export another historical version of the same document.

Webhook configuration is server-owned. Strict delivery is the default and a configured `WEBHOOK_URL` requires an explicitly supplied non-empty `WEBHOOK_SECRET`; there is no public fallback secret. Controlled mode is reserved for the exact Compose receiver `http://receiver:8090/webhook` and is not a general private-network destination setting.

All approval and export routes return `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

| Status | Implemented codes | Meaning |
| --- | --- | --- |
| `400` | `invalid_review`, `invalid_rejection`, `invalid_approval`, `invalid_export`, `webhook_not_configured` | Body was malformed, unknown fields supplied, approval not confirmed, or server webhook destination not configured. |
| `404` | `document_not_found` | The UUID was invalid or no matching document exists. |
| `409` | `invalid_document_transition`, `stale_review_version` | Document not in `needs_review` for approval, or not in `approved`/`exported` for export. |
| `500` | `source_unavailable`, `internal_error` | A safe request could not complete. |

## Implemented Stage 9 operational metrics

The **worker** process optionally serves a Prometheus text exposition (ADR-017). It is a separate listener, configured by `METRICS_ADDR`, and is **empty by default**, so no listener is opened unless an operator asks for one. Configuration refuses an address equal to `API_ADDR`.

| Method and path | Behavior | Response |
| --- | --- | --- |
| `GET /metrics` | Renders the worker instrument set. Any other path or method on this listener is not served. | `200 text/plain; version=0.0.4; charset=utf-8` with `Cache-Control: no-store` and `X-Content-Type-Options: nosniff` |

Instruments:

| Name | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `invoiceflow_process_jobs_total` | counter | `outcome` = `success` / `retry` / `dead_letter` | Finished document processing jobs, counted only after the durable transition committed. |
| `invoiceflow_export_jobs_total` | counter | `outcome` (same values) | Finished webhook export jobs. |
| `invoiceflow_extraction_duration_seconds` | histogram | — | One extraction attempt, successful or not. Bounds bracket the 15 s PDF and 30 s OCR timeouts. |
| `invoiceflow_documents` | gauge | `status` | Documents per document state, read at scrape time. |
| `invoiceflow_jobs` | gauge | `status` | Durable jobs per job status; queue depth is the `ready` series. |

The exposition is aggregate: it contains no document id, hash, storage key, supplier, amount, or provider detail. It has no authentication of its own and reveals traffic volume, so a deployment must bind it to an address that is not publicly reachable.

## Not implemented

No document search, job retry endpoint, payment, multi-tenant billing, user authentication/authorization, or pagination beyond the bounded document list and fixed detail bounds exists. The metrics endpoint carries no per-route latency histogram, tracing, or alerting.
