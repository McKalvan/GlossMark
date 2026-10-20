/* misc stuff for the handheld pages */
var state = { step: 0, loc: null, line: null };

function $(id) {
  return document.getElementById(id);
}

function beep(ok) {
  var a = new Audio(ok ? "ok.wav" : "err.wav");
  a.play();
}

function doIt(e) {
  if (e.keyCode !== 13) return;
  var v = $("scan").value.trim();
  $("scan").value = "";
  process(v);
}

// @gloss implements .ScanConfirm conf=0.85
function process(code) {
  var line = state.line;
  if (state.step === 0) {
    if (code !== line.loc) { beep(false); return; }
    state.step = 1;
    $("msg").innerText = "scan item " + line.item;
  } else {
    if (code !== line.item) { beep(false); return; }
    send("functions.php?a=conf", { id: line.id, qty: line.qty }, next);
  }
  beep(true);
}

function next(resp) {
  state.step = 0;
  state.line = resp.next;
  if (!state.line) { $("msg").innerText = "done"; return; }
  $("msg").innerText = "go to " + state.line.loc;
}

function send(url, data, cb) {
  var x = new XMLHttpRequest();
  x.open("POST", url, true);
  x.setRequestHeader("Content-Type", "application/json");
  x.onload = function () { cb(JSON.parse(x.responseText)); };
  x.send(JSON.stringify(data));
}
