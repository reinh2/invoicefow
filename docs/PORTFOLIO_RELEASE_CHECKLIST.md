# Portfolio Release Checklist

## Product

- [x] Complete upload-to-export flow.
- [x] Three fictional sample invoices.
- [x] Duplicate flow.
- [x] Warning and correction flow.
- [x] Retry/failed state.
- [x] Audit history.

## Engineering

- [x] Go checks.
- [x] PostgreSQL integration tests.
- [x] Frontend tests/build.
- [x] Compose smoke.
- [x] Durable database jobs.
- [x] Safe migrations.
- [x] Idempotent export.
- [x] Separate liveness/readiness.

## Security

- [x] File signatures and limits.
- [x] Server-owned storage keys.
- [x] Safe fixed process invocation.
- [x] Timeouts/output bounds.
- [x] Sanitized logs.
- [x] Signed webhooks.
- [x] AI cannot approve/export.
- [x] No credentials or real documents.

## Presentation

- [x] Business-first README.
- [x] Real runtime screenshots.
- [x] Honest limitations.
- [x] Architecture matches code.
- [x] 60–90 second demo script.

Validated on 24 July 2026 with a disposable PostgreSQL 17 container, a
disposable Node 22 container (the host has no npm executable), a clean named
Compose project, and the separately isolated Compose smoke. The checked
security items apply to the documented loopback, fixed-actor local demo;
authentication, authorization, and production CSRF are explicitly not claimed.
