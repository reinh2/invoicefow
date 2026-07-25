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
| `METRICS_ADDR` | unset, or an address the internet cannot reach | Opens the worker's metrics listener (ADR-017). Empty is the default and opens nothing. The endpoint has no authentication and reveals traffic volume and queue depth, so **never** expose it publicly; configuration refuses an address equal to `API_ADDR`, but that check cannot see the platform's routing. |

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
- a `STORAGE_DIR` that can be discarded. Note that it cannot simply be
  container-local: the API and the worker are separate containers and must share
  it, so in the Compose deployment it is a named volume that the scheduled reset
  removes. Ephemerality comes from the reset, not from the absence of a volume;
- a scheduled restart or reset (daily is reasonable) so the workspace does not
  accumulate whatever strangers upload. `deploy/reset.sh` and
  `deploy/invoiceflow-reset.timer` implement this.

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

**This constraint decides the platform.** There is no S3 storage adapter yet
(it is in the `ROADMAP.md` backlog), so storage is a filesystem both processes
open. Most PaaS offerings attach a disk to exactly one service, which would
force a supervisor process starting both binaries in one container — new code,
and a new failure mode, since nothing would restart the worker if it died. A
single host running the existing Compose files needs none of that.

### Recommended: one host, the committed Compose files, nginx in front

This is the only path that requires no change to the application, because
`docker-compose.yml` already shares `document_data` between the API and the
worker and already publishes the API to loopback only — it was written expecting
a proxy in front of it.

Everything needed is committed:

| File | Purpose |
| --- | --- |
| `docker-compose.public.yml` | Override: turns on `PUBLIC_DEMO`, sets the upload backstop, removes the Postgres volume and every published port except the API's, pins the offline extractor, and refuses to start without an explicit `WEBHOOK_SECRET`. |
| `deploy/nginx/invoiceflow.conf` | TLS termination and the real per-visitor rate limit. |
| `deploy/reset.sh` | Wipes uploads and the database, then restarts. |
| `deploy/invoiceflow-reset.service` / `.timer` | Runs the reset nightly. |

Bring it up with both files, never the base file alone:

```sh
docker compose -f docker-compose.yml -f docker-compose.public.yml up -d --wait
```

Costs: a host and a hostname. Both have free options — an always-free small VM
from a cloud provider, and a free subdomain from a dynamic-DNS service, which
Let's Encrypt will issue a certificate for. Building the image on the host needs
no registry account; a 2 GB machine is enough to run it, and rather more is
comfortable while building the Node and Go stages.

### Rate limiting is not where it looks like it is

`UPLOAD_RATE_PER_MINUTE` identifies a client by transport peer address and
ignores `X-Forwarded-For` on purpose (ADR-016). Behind the proxy every request
therefore arrives with the *same* peer address, so that setting bounds the whole
instance rather than each visitor. It is a backstop. The per-visitor limit lives
in `deploy/nginx/invoiceflow.conf`, where `$binary_remote_addr` is still the real
client: a chatty browse zone and a much stricter zone for `POST /api/v1/documents`,
which is the only expensive, state-creating route.

### The webhook destination

`docker-compose.public.yml` keeps `WEBHOOK_MODE=controlled` and the internal
`receiver` container, so a visitor can exercise the signed-webhook export and see
the retry and audit behaviour — which is a large part of what the project is
demonstrating. This is not a real external destination: it is the exact fixed
internal receiver of ADR-012, it is unreachable from outside the Compose network,
and it verifies the signature, timestamp window, and idempotency key before
accepting anything.

What must change is the secret. `docker-compose.yml` carries
`local-demo-webhook-secret`, which is committed and therefore public. The
override requires `WEBHOOK_SECRET` to be supplied explicitly and refuses to start
without it:

```sh
printf 'WEBHOOK_SECRET=%s
' "$(openssl rand -hex 32)" > .env
```

## Step by step

Assumes a fresh Debian/Ubuntu host with Docker installed, a hostname pointing at
it, and the repository cloned to `/srv/invoiceflow`.

1. **Secret.** In `/srv/invoiceflow`, create `.env` with a generated
   `WEBHOOK_SECRET` (command above). Nothing else belongs in it.
2. **Certificate.** Install nginx and certbot, then issue the certificate for
   the hostname *before* enabling the site — the 443 block will not start
   without it.
3. **Proxy.** Copy `deploy/nginx/invoiceflow.conf` into
   `/etc/nginx/sites-available/`, replace every `DEMO_HOSTNAME` with the real
   hostname, symlink it into `sites-enabled/`, then `nginx -t` and reload. Do
   not skip `nginx -t`: this file has not been validated against a running
   nginx.
4. **Application.** Bring up both Compose files (command above). The first run
   builds the image, which takes a while.
5. **Reset schedule.** Copy `deploy/invoiceflow-reset.service` and
   `deploy/invoiceflow-reset.timer` into `/etc/systemd/system/`, then
   `systemctl daemon-reload` and `systemctl enable --now invoiceflow-reset.timer`.
   Verify with `systemctl list-timers invoiceflow-reset`.
6. **Firewall.** Allow only 22, 80, and 443. Every application port is bound to
   loopback or to the Compose network, but a firewall makes that a property of
   the host rather than of a configuration file.
7. Work through the checklist below.

## After deploying

1. Upload each file in `testdata/` and confirm it reaches `needs_review`.
2. Confirm the shared-demo notice is visible on `/` and `/app`.
3. Confirm `GET /api/v1/config` returns `{"public_demo":true}`.
4. Exceed `UPLOAD_RATE_PER_MINUTE` and confirm the `429` with `Retry-After`.
5. Confirm `/healthz` and `/readyz` answer, and point the platform's health check
   at `/readyz`.
6. Confirm `X-Request-Id` is present on responses and that the access log line
   for a request carries the same id, so a visitor's report can be traced.
7. If `METRICS_ADDR` is set, confirm from outside the platform that the metrics
   address is **not** reachable. If that cannot be proven, leave it unset. With
   the Compose override the port is never published, so it is reachable only as
   `docker compose exec worker wget -qO- 127.0.0.1:9090/metrics`; run that once
   to confirm the endpoint works, and run a scan from outside to confirm it does
   not answer there.
8. Confirm the reset actually erases: upload a document, run
   `systemctl start invoiceflow-reset`, and confirm the document list is empty
   afterwards. An unverified reset is the one failure that turns a demo into a
   place strangers store files.
9. Only once every step above passes, add the URL to the `README.md` header and
   to this document. Until then the repository claims no public instance.

## What is still missing for production

Authentication, authorization, per-visitor isolation, production CSRF defence,
least-privilege database roles, secret management, backups, alerting, and a
stronger process sandbox. Basic metrics now exist (ADR-017) but are opt-in and
must stay private; there is no tracing, no per-route latency histogram, and no
alerting. These are tracked in `ROADMAP.md` and are not provided
by this guide.
