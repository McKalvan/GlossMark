#!/usr/bin/env bash
# @gloss deploys circ:CirculationService scope=file
set -euo pipefail

# @gloss configures circ:ReturnEventBus
configure_bus() {
  echo "@gloss implements circ:Nothing"  # string, not a marker
}
