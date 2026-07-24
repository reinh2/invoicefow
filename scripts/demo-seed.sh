#!/bin/sh
# Drives the running demo through the real fictional flow so the UI has genuine
# content to screenshot or record. It uses only the fictional fixtures in
# testdata/ and only the public API — no manual database edit.
#
#   COMPOSE_PROJECT_NAME=invoiceflow-media API_HOST_PORT=18081 \
#     docker compose up --build --wait
#   API_HOST_PORT=18081 sh scripts/demo-seed.sh
#
# It prints the identifier of one document left in needs_review and one left in
# exported, so a capture step can open both states.
set -eu

base_url="http://127.0.0.1:${API_HOST_PORT:-8080}"

upload() {
  response="$(curl --fail --silent --show-error -F "file=@$1;type=application/pdf" "${base_url}/api/v1/documents")"
  printf '%s' "$response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p'
}

await_review() {
  attempt=0
  while [ "$attempt" -lt 60 ]; do
    state="$(curl --fail --silent --show-error "${base_url}/api/v1/documents/$1" | sed -n 's/.*"status":"\([^"]*\)".*/\1/p')"
    if [ "$state" = "needs_review" ]; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  printf 'document %s never reached needs_review\n' "$1" >&2
  return 1
}

curl --fail --silent --show-error "${base_url}/readyz" >/dev/null

# One document left mid-review, with a human correction saved as version 2.
review_id="$(upload testdata/stage2-fictional-compose.pdf)"
await_review "$review_id"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"base_version":1,"proposal":{"supplier_name":"Fictional Office Goods","invoice_number":"OFFICE-001","issue_date":"2026-07-01","due_date":"2026-07-31","currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Fictional paper — A4, 80 gsm","quantity":"2","unit_price":"10.00","tax_amount":"0.00","total":"20.00"}]}}' \
  "${base_url}/api/v1/documents/${review_id}/human-reviews" >/dev/null

# One document carried all the way through approval and both export routes.
export_id="$(upload testdata/stage2-fictional-compose2.pdf)"
await_review "$export_id"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"base_version":1,"proposal":{"supplier_name":"Fictional Office Goods","invoice_number":"OFFICE-002","issue_date":"2026-07-02","due_date":"2026-08-01","currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Fictional paper — A4, 80 gsm","quantity":"2","unit_price":"10.00","tax_amount":"0.00","total":"20.00"}]}}' \
  "${base_url}/api/v1/documents/${export_id}/human-reviews" >/dev/null
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"version_number":2,"confirm":true}' "${base_url}/api/v1/documents/${export_id}/approve" >/dev/null
curl --fail --silent --show-error "${base_url}/api/v1/documents/${export_id}/export/csv" -o /dev/null
curl --fail --silent --show-error -H 'Content-Type: application/json' -X POST \
  "${base_url}/api/v1/documents/${export_id}/export/webhook" >/dev/null

printf 'needs_review %s\n' "$review_id"
printf 'exported     %s\n' "$export_id"
