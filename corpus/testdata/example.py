"""Breeding engine.

The word @gloss in a docstring is not a marker because it is not in a comment.
"""
from zoo.models import Horse, Stallion, Mare

# @gloss implements impl:StallionProduction
def breed_stallion(sire: Horse, dam: Horse) -> Stallion:
    return Stallion(sire, dam)


# @gloss? represents zoo:Mare conf=0.72 note="agent guess from field names"
class MareRecord:
    foaling_date: str


# @gloss produces zoo:Foal, zoo:FoalBorn
@retry(times=3)
def foal(dam: Mare):
    s = "@gloss implements impl:Nothing"  # a string literal, never a marker
    return s


# @gloss? implements impl:NEW:Gelding note="no concept exists yet"
def geld(horse: Horse):
    pass
