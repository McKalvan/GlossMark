import csv
import io

from flask import Flask, jsonify, request

from core.Manager import Manager
from core.Processor import Processor
from helpers import process_data, handle
from utils.misc import load_cfg, log_it

app = Flask(__name__)
cfg = load_cfg()
mgr = Manager(cfg)


@app.route("/upload", methods=["POST"])
def upload():
    f = request.files["file"]
    rows = list(csv.DictReader(io.StringIO(f.read().decode("utf-8"))))
    out = process_data(rows, cfg)
    log_it("upload", len(rows), len(out))
    return jsonify(out)


@app.route("/run", methods=["POST"])
def run():
    what = request.form.get("what", "1")
    if what == "1":
        res = mgr.doIt()
    else:
        res = mgr.run_batch(request.form.get("cutoff"))
    return jsonify(res)


@app.route("/rma", methods=["POST"])
def rma():
    p = Processor(cfg)
    return jsonify(p.process(request.get_json()))


@app.route("/do", methods=["POST"])
def do():
    return jsonify(handle(request.get_json(), cfg))


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
