"""Listing model for front lights."""
from dataclasses import dataclass


@dataclass(frozen=True)
class LightListing:
    sku: str
    name: str
    lumens: int
    battery_wh: float


# @gloss implements spk:LightListing
def shipping_note(listing: LightListing) -> str:
    if listing.battery_wh > 100:
        return "Ships by ground only: battery over 100 Wh."
    return "Ships with the UN3481 lithium battery label."
