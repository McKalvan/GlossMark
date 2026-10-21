"""Checks a scanned barcode against the part the bin should hold."""
from dataclasses import dataclass


@dataclass(frozen=True)
class ScanResult:
    ok: bool
    reason: str = ""


# @gloss implements spk:ScanVerifier
def verify_scan(expected_sku: str, barcode: str, gtin_to_sku: dict[str, str]) -> ScanResult:
    sku = gtin_to_sku.get(barcode)
    if sku is None:
        return ScanResult(False, f"unknown barcode {barcode}")
    if sku != expected_sku:
        return ScanResult(False, f"scanned {sku}, bin expects {expected_sku}")
    return ScanResult(True)
