from flask import Flask, request

from core.Manager import Manager

app = Flask(__name__)


# @gloss implements wh:WavePlanning
@app.route("/waves", methods=["POST"])
def run():
    """Start a wave for the open orders."""
    return Manager().run_batch(request.json["cutoff"])


# @gloss implements wh:Replenishment
@app.route("/zones/<zone>/fill", methods=["POST"])
def do(zone):
    """Kick off a top up for one zone."""
    return Manager().doIt(zone)


@app.route("/health")
def ok():
    return "ok"
