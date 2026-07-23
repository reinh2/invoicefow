# API Contract

## Implemented operational endpoints

| Method and path | Behavior | Response |
| --- | --- | --- |
| `GET /healthz` | Liveness probe; it does not contact PostgreSQL. | `200 {"status":"ok"}` |
| `GET /readyz` | Readiness probe; performs a bounded PostgreSQL ping. | `200 {"status":"ready"}` or `503 {"status":"not_ready"}` |

The API binds to `127.0.0.1:8080` by default. Compose publishes it only to loopback; set `API_HOST_PORT` or `POSTGRES_HOST_PORT` when either local port is already in use.

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

All review routes return `Cache-Control: no-store` and `X-Content-Type-Options: nosniff` where a response body is emitted.

| Status | Implemented codes | Meaning |
| --- | --- | --- |
| `400` | `invalid_review`, `invalid_rejection` | Body was malformed, unknown fields were supplied, candidate values had the wrong primitive shape/limits, or rejection was not confirmed. |
| `404` | `document_not_found` | The UUID was invalid or no matching document exists. |
| `409` | `invalid_document_transition`, `stale_review_version` | The document no longer permits the action, or another immutable version was saved first. |
| `500` | `source_unavailable`, `internal_error` | A safe request could not complete. |

## Not implemented

No document list, job retry, approval, CSV/webhook export, payment, webhook configuration, user authentication/authorization, pagination beyond the fixed detail bounds, or metrics endpoint exists.
