"""Handheld scanner endpoint: records that items left a bin."""
from dataclasses import dataclass


@dataclass(frozen=True)
class BinPick:
    wave_id: str
    bin_code: str
    sku: str
    quantity: int


# @gloss implements spk:BinPickHandler
def record_pick(stock: dict[tuple[str, str], int], pick: BinPick) -> dict[tuple[str, str], int]:
    key = (pick.bin_code, pick.sku)
    if stock.get(key, 0) < pick.quantity:
        raise ValueError(f"bin {pick.bin_code} holds fewer than {pick.quantity} of {pick.sku}")
    return {**stock, key: stock[key] - pick.quantity}
