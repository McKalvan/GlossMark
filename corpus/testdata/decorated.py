"""Markers above a decorator stack bind to the decorated declaration."""
from dataclasses import dataclass

# @gloss implements circ:ReturnPipeline
@app.route(
    "/return",
    methods=["POST"],  # a ')' in a comment does not close the call
)
@retry(times=3)
def return_endpoint():
    pass


# @gloss implements circ:MarcFieldMapper
@dataclass(frozen=True)
class Shelf:
    size: int


# @gloss implements circ:CoverImageCache
@decorator(
    arg=1)
def shelve():
    pass
