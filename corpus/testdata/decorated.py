"""Markers above a decorator stack bind to the decorated declaration."""
from dataclasses import dataclass

# @gloss implements impl:FoalingPipeline
@app.route(
    "/foal",
    methods=["POST"],  # a ')' in a comment does not close the call
)
@retry(times=3)
def foal_endpoint():
    pass


# @gloss implements impl:HoofStep
@dataclass(frozen=True)
class Shoe:
    size: int


# @gloss implements impl:TailQueue
@decorator(
    arg=1)
def shoe():
    pass
