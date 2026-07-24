# Deploying a public InvoiceFlow demo

This document describes how to run the demo somewhere a portfolio reader can
click it. It is a guide, not a claim: **this repository operates no public
instance and publishes no URL.** Running one is a deliberate act, and the
boundary it crosses is recorded in ADR-016.

## Read this first

A public instance has **no authentication and no authorization**. Every visitor
shares one workspace and can read, correct, approve, reject, and export every
document in it. That is by design for a demo whose only content is fictional and
disposable — and it is unacceptable for anything else.

Do not deploy an instance that:

- holds real invoices, business data, or personal data;
- carries a provider API key you care about;
- shares a database or storage volume with anything else;
- is presented to anyone as a multi-tenant or production service.

## Required configuration

| Variable | Value for a public demo | Why |
| --- | --- | --- |
| `PUBLIC_DEMO` | `true` | Renders the shared-demo notice in the interface. Visitors must not have to read the README to learn the workspace is shared and wiped. |
| `UPLOAD_RATE_PER_MINUTE` | e.g. `10` | Bounds uploads per client address, checked before the body is read. Zero (the default) disables it. |
| `EXTRACTOR` | unset (`fake`) | Keeps the instance offline and free. `openai` would bill a key to anonymous traffic. |
| `WEBHOOK_URL` | unset | Leaves webhook export unconfigured. Do not point a public demo at a real receiver. |
| `WEB_DIR` | `/app/web` | Serves the built interface, same as Compose (ADR-013). |
| `API_ADDR` | `0.0.0.0:8080` | The platform terminates TLS in front of the process. |
| `DATABASE_URL` | an **ephemeral** database | See below. |
| `DEMO_ACTOR` | e.g. `public-demo` | Appears in the audit trail. |

The rate limiter identifies a client by transport peer address and deliberately
ignores `X-Forwarded-For`, because any caller can set it. **Behind a platform
proxy every request may therefore share one peer address**, which makes the
in-process limit close to global rather than per-visitor. Configure the
platform's own rate limiting as the real defence and treat this limiter as a
backstop.

## Disposable data

Use a database and storage the deployment can lose without consequence:

- a small managed Postgres instance created solely for the demo, or a Postgres
  container with **no persistent volume**;
- container-local storage for `STORAGE_DIR` — no mounted volume, so uploads
  disappear with the container;
- a scheduled restart or reset (daily is reasonable) so the workspace does not
  accumulate whatever strangers upload.

Migrations run automatically at process start, so a fresh database needs no
manual step.

## Shape of the deployment

The `Dockerfile` at the repository root already builds both binaries and the web
bundle, and any platform that runs a container image can host it:

- **one web service** running `/app/invoiceflow-api` with the variables above;
- **one worker service** running `/app/invoiceflow-worker` sharing
  `DATABASE_URL` and `STORAGE_DIR`;
- **one Postgres** with no durable volume.

Both processes need the same `STORAGE_DIR`. On a platform where services cannot
share a filesystem, run a single instance of the image that starts both
processes, or attach the same volume to both — otherwise the worker cannot read
the bytes the API stored.

## After deploying

1. Upload each file in `testdata/` and confirm it reaches `needs_review`.
2. Confirm the shared-demo notice is visible on `/` and `/app`.
3. Confirm `GET /api/v1/config` returns `{"public_demo":true}`.
4. Exceed `UPLOAD_RATE_PER_MINUTE` and confirm the `429` with `Retry-After`.
5. Confirm `/healthz` and `/readyz` answer, and point the platform's health check
   at `/readyz`.

## What is still missing for production

Authentication, authorization, per-visitor isolation, production CSRF defence,
least-privilege database roles, secret management, metrics, backups, and a
stronger process sandbox. These are tracked in `ROADMAP.md` and are not provided
by this guide.
