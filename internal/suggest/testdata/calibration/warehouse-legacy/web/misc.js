// Recount one location from the handheld.
// @gloss implements wh:CycleCount
function process(loc) {
  return fetch('/count/' + loc);
}

// Top up a pick face from the handheld.
// @gloss implements wh:Replenishment
function doIt(zone) {
  return fetch('/zones/' + zone + '/fill', { method: 'POST' });
}

function util(x) {
  return x;
}
