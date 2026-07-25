#!/bin/sh
# Wipe and restart the public demo (ADR-016).
#
# The instance is a shared, unauthenticated workspace that strangers upload to.
# Erasing it on a schedule is what makes that acceptable: nothing accumulates,
# and the notice in the interface stays true.
#
# `down -v` removes the document_data volume, which holds the uploaded originals.
# The database has no volume at all in docker-compose.public.yml, so it is gone
# with its container; migrations run again automatically at start.
#
# Usage:
#   INVOICEFLOW_DIR=/srv/invoiceflow sh deploy/reset.sh
#
# Scheduled by deploy/invoiceflow-reset.timer.
set -eu

dir="${INVOICEFLOW_DIR:-/srv/invoiceflow}"
cd "$dir"

compose="docker compose -f docker-compose.yml -f docker-compose.public.yml"

# WEBHOOK_SECRET is required by the public override; the .env file next to the
# compose files supplies it. Failing here is correct: a demo that restarts
# without its secret would refuse to start anyway, and doing so loudly is better
# than a half-running instance.
printf 'invoiceflow: resetting demo in %s\n' "$dir"

$compose down -v --remove-orphans
$compose up -d --wait

# The image is rebuilt only by a deploy, not by a reset, so prune just the
# dangling layers a rebuild left behind. Anything still referenced is untouched.
docker image prune -f >/dev/null

printf 'invoiceflow: demo reset complete\n'
