# API Contract — Draft

Replace this with the real implemented contract.

## Conventions

- Prefix: `/api/v1`
- RFC 3339 UTC timestamps.
- Exact money: minor units plus currency.
- Opaque server-generated IDs.
- Stable JSON errors:
  ```json
  {
    "error": {
      "code": "stable_code",
      "message": "Human-readable message",
      "details": {}
    }
  }
  ```

## Candidate routes

```text
POST   /api/v1/documents
GET    /api/v1/documents
GET    /api/v1/documents/{id}
GET    /api/v1/documents/{id}/file
POST   /api/v1/documents/{id}/retry
POST   /api/v1/documents/{id}/reviews
POST   /api/v1/documents/{id}/approve
POST   /api/v1/documents/{id}/reject
POST   /api/v1/documents/{id}/exports
GET    /api/v1/documents/{id}/audit

GET    /healthz
GET    /readyz
GET    /metrics
```

Approval references an exact review version. Export references an exact approved version.
