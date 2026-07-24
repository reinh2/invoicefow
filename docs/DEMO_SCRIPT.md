# InvoiceFlow — 75-second demo script

Use an isolated local Compose demo and only the fictional fixtures in
`testdata/`. The script contains no measured performance, customer, accuracy,
or compliance claim.

## Preparation

```sh
COMPOSE_PROJECT_NAME=invoiceflow-media API_HOST_PORT=18081 \
POSTGRES_HOST_PORT=15433 RECEIVER_HOST_PORT=18091 docker compose up --build --wait
API_HOST_PORT=18081 sh scripts/demo-seed.sh
```

The seed command prints a corrected Cedarline review, an image-OCR review, and
an exported Aurora flow. Keep the Compose project running while presenting.
Tear it down with the same environment values and `docker compose down -v` after
capture.

To regenerate the checked-in screenshots, install the optional capture browser
once and pass the printed image-review ID:

```sh
cd web && npm ci && npx playwright install chromium
BASE_URL=http://127.0.0.1:18081 REVIEW_DOC_ID=<needs_review_image-id> \
npm run capture:media
```

The script writes the checked-in landing video, poster, and review still under
`public/media/`. It opens only the local demo URLs above.

## Narration (about 75 seconds)

**0–10 seconds — product boundary.** “InvoiceFlow turns a PDF, JPEG, or PNG
invoice into a structured proposal. It does not pay invoices or approve them
automatically: a person stays responsible for the final record.”

**10–23 seconds — intake and durable work.** Open `/app` and point to the
upload state. “The API validates the name, declared type, and file signature,
computes a SHA-256 identity, stores the original under a server-owned key, and
creates the document, audit event, and processing job together.”

**23–38 seconds — review a real warning.** Open the `warning` ID printed by
`demo-seed.sh`. “This fictional Cedarline invoice is intentionally inconsistent:
the server shows `subtotal_tax_total_mismatch` beside the original. The warning
is a check, not an automatic accounting decision.”

**38–52 seconds — correction and immutable version.** Open the `needs_review`
ID. “Saving a correction creates version two; it does not overwrite the model
proposal. The audit history preserves both versions, and approval must target an
exact version after an explicit confirmation.”

**52–66 seconds — image OCR and export.** Open the `exported` ID. “This
fictional PNG uses the local image-OCR route. Its approved version was exported
to deterministic CSV and a signed webhook job. The webhook carries a stable
idempotency key, so a receiver can safely deduplicate retries.”

**66–75 seconds — honest close.** “The default demo is offline and uses only
fictional documents. It intentionally has no authentication, no live model
provider, no payment or banking integration, and no raster OCR for scanned
PDFs.”
