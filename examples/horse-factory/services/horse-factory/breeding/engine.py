"""Breeding engine: turns a sire and a dam into foals."""
from models import Horse, Stallion, Mare


# @gloss implements .StallionProduction
# @gloss produces zoo:Stallion
def foal_male(sire: Horse, dam: Mare) -> Stallion:
    return Stallion(sire=sire, dam=dam)


# @gloss? produces zoo:Donkey conf=0.55 note="mule breeding path added for the county fair"
def foal_mule(jack, dam: Mare):
    return dam.cross(jack)


# @gloss implements .FoalingPipeline
def run_pipeline(batch):
    for mare in batch:
        yield mare.foal()


# @gloss? implements impl:NEW:Gelding note="castration step has no concept yet"
def geld(horse: Horse):
    horse.gelded = True
