#!/bin/sh

# Top up every zone overnight.
# @gloss implements wh:Replenishment
topup_all() {
  python -c 'from core.Manager import Manager; Manager().doIt("*")'
}

log() {
  echo "$@"
}
