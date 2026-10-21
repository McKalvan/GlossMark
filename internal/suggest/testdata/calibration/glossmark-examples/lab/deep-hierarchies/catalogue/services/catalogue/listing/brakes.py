"""Listing model for hydraulic disc brakes."""
from dataclasses import dataclass

ROTOR_SIZES_MM = (140, 160, 180, 203)


@dataclass(frozen=True)
class BrakeListing:
    sku: str
    name: str
    fluid: str
    max_rotor_mm: int


# @gloss implements spk:BrakeListing
def rotor_options(listing: BrakeListing) -> list[int]:
    return [size for size in ROTOR_SIZES_MM if size <= listing.max_rotor_mm]
