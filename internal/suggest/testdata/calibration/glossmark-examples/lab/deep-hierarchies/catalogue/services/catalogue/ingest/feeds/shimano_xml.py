"""Shimano dealer feed, XML schema v2."""
from __future__ import annotations

import xml.etree.ElementTree as ET
from dataclasses import dataclass

CATEGORY_MAP = {
    "RD-DI2": "electronic-rear-derailleur",
    "FD-DI2": "electronic-front-derailleur",
    "CS": "cassette",
}


@dataclass(frozen=True)
class FeedItem:
    sku: str
    category: str
    name: str
    price_cents: int


# @gloss implements spk:ShimanoXmlImporter
# @gloss produces cyc:ElectronicRearDerailleur, cyc:ElectronicFrontDerailleur, cyc:Cassette
def parse_feed(document: bytes) -> list[FeedItem]:
    root = ET.fromstring(document)
    items = []
    for node in root.iterfind("./products/product"):
        series = node.findtext("series", "")
        category = CATEGORY_MAP.get(series)
        if category is None:
            continue
        items.append(
            FeedItem(
                sku=node.attrib["sku"],
                category=category,
                name=node.findtext("name", "").strip(),
                price_cents=round(float(node.findtext("dealerPrice", "0")) * 100),
            )
        )
    return items
