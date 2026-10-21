"""Generic CSV supplier feeds (brakes from smaller distributors)."""
from __future__ import annotations

import csv
import io
from dataclasses import dataclass

REQUIRED = ("sku", "type", "name", "price")


class FeedRowError(ValueError):
    pass


@dataclass(frozen=True)
class BrakeRow:
    sku: str
    actuation: str
    name: str
    price_cents: int


# @gloss implements spk:CsvRowParser
def parse_row(row: dict[str, str]) -> BrakeRow:
    missing = [key for key in REQUIRED if not row.get(key)]
    if missing:
        raise FeedRowError(f"missing columns: {', '.join(missing)}")
    return BrakeRow(row["sku"], row["type"].lower(), row["name"].strip(), round(float(row["price"]) * 100))


# @gloss implements spk:CsvFeedImporter
# @gloss produces cyc:HydraulicDiscBrake
def import_csv(text: str) -> list[BrakeRow]:
    reader = csv.DictReader(io.StringIO(text))
    return [row for row in map(parse_row, reader) if row.actuation == "hydraulic"]
