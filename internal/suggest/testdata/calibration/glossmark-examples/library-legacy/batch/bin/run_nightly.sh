#!/bin/sh
# @gloss implements .NightlyBatch scope=file conf=0.85
# run_nightly.sh - the nightly circulation batch. cron at 01:30 until 2024;
# since the cloud move it is the command of the circ-nightly ECS scheduled task.
# Order matters: fines before notices, so a notice shows today's balance.
set -eu

BATCH_DIR=$(cd "$(dirname "$0")/.." && pwd)
CIRC_CONFIG_DIR=${CIRC_CONFIG_DIR:-/etc/circ}
LOG_DIR=${LOG_DIR:-/var/log/circ}
TODAY=$(date +%Y-%m-%d)
export CIRC_CONFIG_DIR

if [ -f "$CIRC_CONFIG_DIR/batch.env" ]; then
    # shellcheck disable=SC1091
    . "$CIRC_CONFIG_DIR/batch.env"
fi

log() {
    printf '%s %s\n' "$(date '+%H:%M:%S')" "$*" | tee -a "$LOG_DIR/nightly-$TODAY.log"
}

log "nightly batch start"
ruby "$BATCH_DIR/assess_fines.rb" --date "$TODAY" >>"$LOG_DIR/nightly-$TODAY.log" 2>&1
log "fines assessed"
perl "$BATCH_DIR/overdue_notices.pl" --date "$TODAY" >>"$LOG_DIR/nightly-$TODAY.log" 2>&1
log "overdue notices sent"
"$BATCH_DIR/bin/expire_holds.sh" "$TODAY" >>"$LOG_DIR/nightly-$TODAY.log" 2>&1
log "holds expired"

find "$LOG_DIR" -name 'nightly-*.log' -mtime +90 -delete
log "nightly batch done"
