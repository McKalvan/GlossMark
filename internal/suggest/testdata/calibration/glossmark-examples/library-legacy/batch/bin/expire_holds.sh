#!/bin/sh
# @gloss implements .HoldExpiryJob scope=file conf=0.85
# expire_holds.sh - expire trapped holds nobody picked up within pickup_days,
# and re-trap the copy for the next waiting hold (sp_expire_holds does both).
set -eu

AS_OF=${1:-$(date +%Y-%m-%d)}

psql --no-psqlrc --quiet --tuples-only \
    --host "${CIRC_DB_HOST:-localhost}" --username "${CIRC_DB_USER:-circ_batch}" "${CIRC_DB_NAME:-circ}" \
    --command "SELECT expired, retrapped FROM sp_expire_holds(DATE '$AS_OF');" |
while read -r expired _ retrapped; do
    echo "$AS_OF expire_holds: $expired expired, $retrapped re-trapped"
done
