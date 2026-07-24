#!/bin/sh
set -eu

project="${COMPOSE_PROJECT_NAME:-invoiceflow-smoke-${PPID}}"
export COMPOSE_PROJECT_NAME="$project"
export API_HOST_PORT="${SMOKE_API_PORT:-18080}"
export POSTGRES_HOST_PORT="${SMOKE_DB_PORT:-15432}"
export RECEIVER_HOST_PORT="${SMOKE_RECEIVER_PORT:-18090}"

tmp_dir="$(mktemp -d)"
cleanup() {
	docker compose down -v --remove-orphans >/dev/null 2>&1 || true
	rm -rf "$tmp_dir"
}
trap cleanup EXIT

docker compose up --build --wait

port="$API_HOST_PORT"
base_url="http://127.0.0.1:${port}"
receiver_url="http://127.0.0.1:${RECEIVER_HOST_PORT}"

curl --fail --silent --show-error "${base_url}/healthz" >/dev/null
curl --fail --silent --show-error "${base_url}/readyz" >/dev/null
curl --fail --silent --show-error "${receiver_url}/healthz" >/dev/null

# Stage 6: the demo serves the real application, not an API-only backend.
curl --fail --silent --show-error -D "$tmp_dir/shell.headers" "${base_url}/" -o "$tmp_dir/shell.html"
grep -q '<div id="root">' "$tmp_dir/shell.html"
grep -qi 'content-security-policy:.*default-src .self.' "$tmp_dir/shell.headers"
grep -qi 'x-content-type-options: nosniff' "$tmp_dir/shell.headers"
grep -qi 'cache-control: no-store' "$tmp_dir/shell.headers"
# A client-routed path loads the same shell; an unknown server route must not.
curl --fail --silent --show-error "${base_url}/app" | grep -q '<div id="root">'
curl --silent --show-error "${base_url}/api/v1/unknown" | grep -q '"code":"route_not_found"'
! curl --silent --show-error "${base_url}/assets/index-absent.js" | grep -q '<div id="root">'

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

# Save human correction version 2
curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"base_version":1,"proposal":{"supplier_name":"Fictional Compose Vendor","invoice_number":"COMPOSE-001","issue_date":"2026-07-23","currency":"USD","subtotal":"20.00","tax_amount":"4.00","total":"24.00","line_items":[{"description":"Fictional service","quantity":"2","unit_price":"10.00","tax_amount":"0.00","total":"20.00"}]}}' "${base_url}/api/v1/documents/${document_id}/human-reviews" | grep -q '"version_number":2'

versions="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM invoice_versions WHERE document_id='${document_id}' AND source='human_review'")"
test "$versions" = "1"
saved_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='human_review_saved'")"
test "$saved_audit" = "1"

# Stage 5: Approve exact version 2
approve_res="$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"version_number":2,"confirm":true}' "${base_url}/api/v1/documents/${document_id}/approve")"
printf '%s' "$approve_res" | grep -q '"status":"approved"'
approved_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='document_approved'")"
test "$approved_audit" = "1"

# Stage 5: CSV Export
curl --fail --silent --show-error "${base_url}/api/v1/documents/${document_id}/export/csv" -o "$tmp_dir/csv.first"
grep -q "Fictional Compose Vendor" "$tmp_dir/csv.first"
curl --fail --silent --show-error -D "$tmp_dir/csv.headers" "${base_url}/api/v1/documents/${document_id}/export/csv" -o "$tmp_dir/csv.second"
cmp "$tmp_dir/csv.first" "$tmp_dir/csv.second"
grep -qi 'X-InvoiceFlow-CSV-Format: csv-v1' "$tmp_dir/csv.headers"
grep -qi "Content-Disposition: attachment; filename=\"invoice-${document_id}-v2.csv\"" "$tmp_dir/csv.headers"
csv_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='csv_exported'")"
test "$csv_audit" = "1"

# Stage 5: Webhook Export (server configured destination)
export_res="$(curl --fail --silent --show-error -H 'Content-Type: application/json' -X POST "${base_url}/api/v1/documents/${document_id}/export/webhook")"
printf '%s' "$export_res" | grep -q '"status":"pending"'
export_res_again="$(curl --fail --silent --show-error -H 'Content-Type: application/json' -X POST "${base_url}/api/v1/documents/${document_id}/export/webhook")"
test "$(printf '%s' "$export_res" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')" = "$(printf '%s' "$export_res_again" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
export_key="$(printf '%s' "$export_res" | sed -n 's/.*"idempotency_key":"\([^"]*\)".*/\1/p')"
db_export_key="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT idempotency_key FROM exports WHERE document_id='${document_id}' AND export_type='webhook'")"
test -n "$export_key"
test "$export_key" = "$db_export_key"
enqueued_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='export_enqueued'")"
test "$enqueued_audit" = "1"

# Wait for worker to deliver webhook export
attempt=0
while [ "$attempt" -lt 30 ]; do
  export_status="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT status FROM exports WHERE document_id='${document_id}' AND export_type='webhook'")"
  if [ "$export_status" = "succeeded" ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
test "${export_status:-}" = "succeeded"

webhook_attempt_projection="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT e.attempts::text || ':' || j.attempts::text || ':' || (e.next_attempt_at IS NULL)::text || ':' || (j.next_attempt_at IS NULL)::text FROM exports e JOIN jobs j ON j.id=e.job_id WHERE e.document_id='${document_id}' AND e.export_type='webhook'")"
test "$webhook_attempt_projection" = "1:1:true:true"

webhook_audit="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT count(*) FROM audit_events WHERE document_id='${document_id}' AND action='webhook_exported'")"
test "$webhook_audit" = "1"
receiver_stats="$(curl --fail --silent --show-error "${receiver_url}/stats")"
printf '%s' "$receiver_stats" | grep -q '"validated_count":1'
printf '%s' "$receiver_stats" | grep -q '"idempotency_count":1'
printf '%s' "$receiver_stats" | grep -q '"last_idempotency_key":"'"$export_key"'"'

# Test Rejection flow on a 2nd fictional document
doc2_res="$(curl --fail --silent --show-error -F 'file=@testdata/stage2-fictional-compose2.pdf;type=application/pdf' "${base_url}/api/v1/documents")"
doc2_id="$(printf '%s' "$doc2_res" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')"
test -n "$doc2_id"

attempt=0
while [ "$attempt" -lt 30 ]; do
  doc2_status="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT status FROM documents WHERE id='${doc2_id}'")"
  if [ "$doc2_status" = "needs_review" ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"confirm":true}' "${base_url}/api/v1/documents/${doc2_id}/reject" >/dev/null
rejected="$(docker compose exec -T postgres psql -U invoiceflow -d invoiceflow -Atc "SELECT status || ':' || (SELECT count(*)::text FROM audit_events WHERE document_id='${doc2_id}' AND action='document_rejected') FROM documents WHERE id='${doc2_id}'")"
test "$rejected" = "rejected:1"

printf '%s\n' 'Compose Stage 6 smoke passed.'
