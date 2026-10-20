"""Splits a fired order into one kitchen ticket per station."""

from collections import defaultdict
from datetime import datetime
from uuid import uuid4

from ordering.models import Allergen, KitchenTicket, MenuItem, Order, Station, TicketLine


# @gloss implements .TicketRouter conf=0.82
def route_ticket(order: Order, items: dict, flagged: frozenset[Allergen],
                 open_stations: frozenset[Station] | None = None) -> list[KitchenTicket]:
    by_station: dict[Station, list[TicketLine]] = defaultdict(list)
    alerts: dict[Station, set[Allergen]] = defaultdict(set)
    for line in order.lines:
        item: MenuItem = items[line.menu_item_id]
        station = Station(item.station)
        if open_stations is not None and station not in open_stations:
            station = Station.PASS
        by_station[station].append(
            TicketLine(name=item.name, quantity=line.quantity, modifiers=line.modifiers, seat=line.seat)
        )
        alerts[station] |= (item.allergens | item.may_contain) & flagged
    fired_at = order.fired_at or datetime.now().astimezone()
    return [
        KitchenTicket(
            id=uuid4(),
            order_id=order.id,
            station=station,
            table=order.table,
            lines=lines,
            allergen_alert=frozenset(alerts[station]),
            fired_at=fired_at,
        )
        for station, lines in sorted(by_station.items())
    ]
