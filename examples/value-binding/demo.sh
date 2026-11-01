#!/usr/bin/env bash
# The sixty-second demo (docs/DEMO.md): a constant becomes a governed business
# rule. The example beside this script is copied into a temporary git
# repository and committed; nothing here is modified.
#
#   1. gloss bind reads AUTO_APPROVE_LIMIT_EUR from settings.py and writes the
#      ontology entry, the marker and the baseline.
#   2. Someone changes the constant. gloss drift reports the drifted value;
#      gloss diff goes HARD and names the owner.
#   3. The business agrees: gloss bind again moves value: and the baseline.
#
# Usage: examples/value-binding/demo.sh            (builds gloss from this tree)
#        GLOSS=/path/to/gloss examples/value-binding/demo.sh
set -euo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
if [ -z "${GLOSS:-}" ]; then
  (cd "$ROOT" && go build -o "$TMP/gloss" ./cmd/gloss)
  GLOSS=$TMP/gloss
fi
export GLOSS_ACTOR=${GLOSS_ACTOR:-human:demo}
SETTINGS=services/assessment/assessment/settings.py

step() { printf '\n\033[1m== %s\033[0m\n' "$*"; }
show() { printf '\033[2m$ %s\033[0m\n' "$*"; "$@"; }
try()  { printf '\033[2m$ %s\033[0m\n' "$*"; "$@" || true; }

cp -r "$HERE" "$TMP/ws"
cd "$TMP/ws"
rm -f demo.sh
git init -q
git add -A
git -c user.name=demo -c user.email=demo@example.invalid commit -qm "before: the limit is a plain constant"

step "0. A plain constant"
show grep -n AUTO_APPROVE_LIMIT_EUR "$SETTINGS"

step "1. gloss bind: the constant is the business rule claim:AutoApprovalLimit"
show "$GLOSS" bind "$SETTINGS:AUTO_APPROVE_LIMIT_EUR" --as tide:AutoApprovalRule --publish-as claim:AutoApprovalLimit --value "5000 EUR"
show "$GLOSS" check
show git --no-pager diff --stat
git add -A
git -c user.name=demo -c user.email=demo@example.invalid commit -qm "bind AUTO_APPROVE_LIMIT_EUR to claim:AutoApprovalLimit"

step "2. Someone raises the limit in the code alone"
sed -i.bak 's/AUTO_APPROVE_LIMIT_EUR = 5_000/AUTO_APPROVE_LIMIT_EUR = 7_500/' "$SETTINGS" && rm -f "$SETTINGS.bak"
show git --no-pager diff --no-color -- "$SETTINGS"
try "$GLOSS" drift --fail-on value-drift
try "$GLOSS" diff --base HEAD

step "3. The business agrees to 7500: gloss bind again moves value: and the baseline"
show "$GLOSS" bind "$SETTINGS:AUTO_APPROVE_LIMIT_EUR" --as tide:AutoApprovalRule --value "7500 EUR"
show "$GLOSS" accept --stale "$SETTINGS"
show "$GLOSS" drift
try "$GLOSS" diff --base HEAD

step "Done. The change is one diff of the code, the ontology and the baseline, gated HARD for claims-decisioning."
