# Architecture — Initial Target

This begins as a design target. Update it to describe the implementation that actually exists.

```text
Browser
  |
React/Vite UI
  |
Go API -------------------- File storage adapter
  |
PostgreSQL
  |
Durable job tables
  |
Go worker
  |--- PDF text adapter
  |--- OCR adapter
  `--- Structured extractor provider
  |
Immutable extraction/review version
  |
Human approval
  |--- CSV
  `--- Signed webhook
```

## Preferred repository layout

```text
cmd/
  api/
  worker/
internal/
  app/
  documents/
  invoices/
  processing/
  extraction/
  review/
  exports/
  audit/
  platform/
db/migrations/
web/
docs/
scripts/
```

## Trust boundaries

Untrusted:

- browser requests;
- uploaded files;
- filenames and MIME claims;
- extracted text;
- OCR output;
- model output;
- webhook targets/responses.

Trusted authority is server-owned state in PostgreSQL.

## Transaction boundaries

Expected atomic operations:

- document + upload audit + processing job;
- job claim + attempt start;
- extraction version + warnings + state + audit;
- review version + edit audit;
- exact approval + audit;
- export enqueue + audit;
- export result + final state/audit.

Network/process work must occur outside long-held transactions.

## Open Stage 0 decisions

- demo authentication model;
- PDF preview library;
- OCR tool/container availability;
- review version model;
- job lease algorithm;
- evidence representation;
- CSV schema;
- webhook signature format.
