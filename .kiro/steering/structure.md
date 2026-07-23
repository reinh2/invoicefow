---
inclusion: always
---

# Structure

```text
cmd/api/
cmd/worker/
internal/app/
internal/documents/
internal/invoices/
internal/processing/
internal/extraction/
internal/review/
internal/exports/
internal/audit/
internal/platform/
db/migrations/
web/
docs/
```

Keep domain rules out of HTTP handlers and React components. Put provider-specific behavior behind adapters. Do not create processing microservices.
