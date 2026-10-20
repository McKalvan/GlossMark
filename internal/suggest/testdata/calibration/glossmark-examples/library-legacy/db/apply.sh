#!/bin/sh
# Apply schema files then (re)create procedures. Idempotent for procs only.
set -eu
DB=${1:-circ}
for f in "$(dirname "$0")"/schema/*.sql; do
    psql --no-psqlrc -v ON_ERROR_STOP=1 -d "$DB" -f "$f"
done
for f in "$(dirname "$0")"/procs/*.sql; do
    psql --no-psqlrc -v ON_ERROR_STOP=1 -d "$DB" -f "$f"
done
