#!/usr/bin/env bash
# @gloss documents gmi:HtmlViewer, gmi:ReviewTerminal scope=file
#
# Regenerates the README screenshots in docs/img/ from the glossmark-examples
# showcase, using the gloss built from this tree (not the release the showcase
# pins in its GLOSS_REF): the viewer's #view= and #open= hash and the review
# terminal's --format frame are newer than v0.2.0.
#
# Needs: go, chromium (CHROMIUM=), freeze (FREEZE=; go install
# github.com/charmbracelet/freeze@latest) and a checkout of
# https://github.com/McKalvan/glossmark-examples beside this repository
# (GLOSS_EXAMPLES= to point elsewhere).
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
EXAMPLES=${GLOSS_EXAMPLES:-$ROOT/../glossmark-examples}
CHROMIUM=${CHROMIUM:-chromium}
FREEZE=${FREEZE:-$(go env GOPATH)/bin/freeze}
OUT=$ROOT/docs/img
SHOWCASE=$EXAMPLES/showcase

[ -d "$SHOWCASE/clinic-booking" ] || { echo "no showcase at $SHOWCASE: clone glossmark-examples beside this repo or set GLOSS_EXAMPLES" >&2; exit 1; }
command -v "$CHROMIUM" >/dev/null || { echo "chromium not found: set CHROMIUM=" >&2; exit 1; }
[ -x "$FREEZE" ] || { echo "freeze not found at $FREEZE: go install github.com/charmbracelet/freeze@latest" >&2; exit 1; }

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
(cd "$ROOT" && go build -o "$TMP/gloss" ./cmd/gloss)
mkdir -p "$OUT"

# shot NAME WIDTH HEIGHT SCALE URL: one headless screenshot.
shot() {
  "$CHROMIUM" --headless=new --no-sandbox --disable-gpu --hide-scrollbars \
    --force-device-scale-factor="$4" --window-size="$2,$3" --virtual-time-budget=4000 \
    --screenshot="$OUT/$1.png" "$5" >/dev/null 2>&1
  [ -s "$OUT/$1.png" ] || { echo "chromium wrote nothing for $1" >&2; exit 1; }
}

# The viewer: a clinic-booking feature slice in the engineering view, and the
# business view of the same graph with every box open.
(cd "$SHOWCASE/clinic-booking" && "$TMP/gloss" view --inline -o "$TMP/clinic.html" >/dev/null)
shot viewer-slice 1440 900 1 "file://$TMP/clinic.html#view=engineering&slice=clinic:Booking"
shot viewer-business 1440 900 1 "file://$TMP/clinic.html#view=business&open=all"

# The review terminal: one frame of the kitchen-agents queue, rendered to SVG
# by freeze and rasterised at 2x. A fixed actor keeps the header reproducible.
(cd "$SHOWCASE/kitchen-agents" && GLOSS_ACTOR=human:reviewer "$FREEZE" \
  --execute "$TMP/gloss review --format frame --size 120x30" \
  --output "$TMP/review.svg" --window --padding 14,20 --font.family Hack --font.size 11 --theme dracula >/dev/null)
W=$(grep -o 'width="[0-9.]*"' "$TMP/review.svg" | head -1 | tr -dc 0-9.)
H=$(grep -o 'height="[0-9.]*"' "$TMP/review.svg" | head -1 | tr -dc 0-9.)
shot review-terminal "${W%.*}" "${H%.*}" 2 "file://$TMP/review.svg"

ls -l "$OUT"/*.png
