"""Fulfilment service entry point: releases paid orders to picking."""
from dataclasses import dataclass


@dataclass(frozen=True)
class Order:
    order_id: str
    lines: tuple[tuple[str, int], ...]
    paid: bool


# @gloss implements spk:FulfilmentEngine
def release(orders: list[Order]) -> list[Order]:
    return [order for order in orders if order.paid and order.lines]
