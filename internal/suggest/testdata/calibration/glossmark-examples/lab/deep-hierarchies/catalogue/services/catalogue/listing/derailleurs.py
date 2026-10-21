"""Listing model for electronic rear derailleurs."""
from dataclasses import dataclass


@dataclass(frozen=True)
class DerailleurListing:
    sku: str
    name: str
    speeds: int
    max_cog_teeth: int
    battery: str


# @gloss implements spk:DerailleurListing
def compatibility_note(listing: DerailleurListing, cassette_max_cog: int) -> str:
    if cassette_max_cog > listing.max_cog_teeth:
        return f"Not compatible: largest cog {cassette_max_cog}T exceeds {listing.max_cog_teeth}T."
    return f"Compatible with {listing.speeds}-speed cassettes up to {listing.max_cog_teeth}T."
