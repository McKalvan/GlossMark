#!/usr/bin/env bash
# @gloss deploys impl:HorseFactoryService scope=file
set -euo pipefail

# @gloss configures impl:FoalEventBus
configure_bus() {
  echo "@gloss implements impl:Nothing"  # string, not a marker
}
