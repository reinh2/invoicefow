#!/bin/sh
set -eu

port="${API_HOST_PORT:-8080}"
base_url="http://127.0.0.1:${port}"

curl --fail --silent --show-error "${base_url}/healthz" >/dev/null
curl --fail --silent --show-error "${base_url}/readyz" >/dev/null
response="$(curl --fail --silent --show-error -F 'file=@testdata/stage2-fictional-compose.pdf;type=application/pdf' "${base_url}/api/v1/documents")"
document_id="$(printf '%s' "$response" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
test -n "$document_id"

attempt=0
while [ "$attempt" -lt 30 ]; do
  status="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT status FROM documents WHERE id='${document_id}'")"
  if [ "$status" = "needs_review" ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
test "${status:-}" = "needs_review"
snapshot="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT rounding_policy_version || ':' || total_minor::text FROM invoice_versions WHERE document_id='${document_id}'")"
test "$snapshot" = "money-v1:2400"
audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='processing_completed'")"
test "$audit" = "1"
detail="$(curl --fail --silent --show-error "${base_url}/api/v1/documents/${document_id}")"
printf '%s' "$detail" | grep -q '"status":"needs_review"'
curl --fail --silent --show-error "${base_url}/api/v1/documents/${document_id}/source" >/dev/null
curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"base_version":1,"proposal":{"supplier_name":"Fictional Compose Vendor","invoice_number":"COMPOSE-001","issue_date":"2026-07-23","currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Fictional service","quantity":"2","unit_price":"10.00","tax_amount":"0.00","total":"20.00"}]}}' "${base_url}/api/v1/documents/${document_id}/human-reviews" | grep -q '"version_number":2'
versions="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM invoice_versions WHERE document_id='${document_id}' AND source='human_review'")"
test "$versions" = "1"
saved_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='human_review_saved'")"
test "$saved_audit" = "1"
curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"confirm":true}' "${base_url}/api/v1/documents/${document_id}/reject" >/dev/null
rejected="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT status || ':' || (SELECT count(*)::text FROM audit_events WHERE document_id='${document_id}' AND action='document_rejected') FROM documents WHERE id='${document_id}'")"
test "$rejected" = "rejected:1"
printf '%s\n' 'Compose Stage 4 smoke passed.'
