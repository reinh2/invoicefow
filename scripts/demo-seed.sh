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
  content_type="${2:-application/pdf}"
  response="$(curl --fail --silent --show-error -F "file=@$1;type=${content_type}" "${base_url}/api/v1/documents")"
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

# The Cedarline document is left mid-review. Its extraction warns that subtotal
# plus tax does not equal the total; the saved human correction (version 2)
# reconciles it, so the review screen shows a warning and a fix side by side.
review_id="$(upload testdata/fixture-cedarline-services.pdf)"
await_review "$review_id"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"base_version":1,"proposal":{"supplier_name":"Cedarline Services LLC","invoice_number":"CEDAR-3390","issue_date":"2026-06-20","due_date":"2026-07-20","currency":"USD","subtotal":"250.00","tax_amount":"47.50","total":"297.50","line_items":[{"description":"Managed hosting, monthly","quantity":"10","unit_price":"15.00","tax_amount":"0.00","total":"150.00"},{"description":"On-site support, hours","quantity":"4","unit_price":"25.00","tax_amount":"0.00","total":"100.00"}]}}' \
  "${base_url}/api/v1/documents/${review_id}/human-reviews" >/dev/null

# The Aurora document is carried all the way through approval and both export
# routes, so the demo has a clean invoice in the exported state.
export_id="$(upload testdata/fixture-aurora-stationery.pdf)"
await_review "$export_id"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"base_version":1,"proposal":{"supplier_name":"Aurora Stationery Co.","invoice_number":"AURORA-1042","issue_date":"2026-06-15","due_date":"2026-07-15","currency":"USD","subtotal":"80.00","tax_amount":"6.40","total":"86.40","line_items":[{"description":"A4 copy paper, 80 gsm (5 reams)","quantity":"5","unit_price":"6.00","tax_amount":"0.00","total":"30.00"},{"description":"Gel ink pens, box of 12","quantity":"3","unit_price":"8.00","tax_amount":"0.00","total":"24.00"},{"description":"Mesh desk organizer","quantity":"2","unit_price":"13.00","tax_amount":"0.00","total":"26.00"}]}}' \
  "${base_url}/api/v1/documents/${export_id}/human-reviews" >/dev/null
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"version_number":2,"confirm":true}' "${base_url}/api/v1/documents/${export_id}/approve" >/dev/null
curl --fail --silent --show-error "${base_url}/api/v1/documents/${export_id}/export/csv" -o /dev/null
curl --fail --silent --show-error -H 'Content-Type: application/json' -X POST \
  "${base_url}/api/v1/documents/${export_id}/export/webhook" >/dev/null

# The Meridian image document is left mid-review. It arrives as a scanned-style
# image and runs through the OCR path, so its original renders inline in the
# review screen's source panel — the state a product screenshot should show.
image_review_id="$(upload testdata/fixture-meridian-supplies.png image/png)"
await_review "$image_review_id"
curl --fail --silent --show-error -H 'Content-Type: application/json' \
  -d '{"base_version":1,"proposal":{"supplier_name":"Meridian Office Supplies","invoice_number":"MERIDIAN-2087","issue_date":"2026-06-18","due_date":"2026-07-18","currency":"USD","subtotal":"63.00","tax_amount":"5.04","total":"68.04","line_items":[{"description":"Ballpoint pens, box of 50","quantity":"4","unit_price":"9.00","tax_amount":"0.00","total":"36.00"},{"description":"Sticky notes, pack of 12","quantity":"6","unit_price":"4.50","tax_amount":"0.00","total":"27.00"}]}}' \
  "${base_url}/api/v1/documents/${image_review_id}/human-reviews" >/dev/null

printf 'needs_review       %s\n' "$review_id"
printf 'needs_review_image %s\n' "$image_review_id"
printf 'exported           %s\n' "$export_id"
