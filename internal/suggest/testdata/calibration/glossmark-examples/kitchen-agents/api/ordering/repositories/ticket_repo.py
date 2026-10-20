import json
from datetime import datetime
from uuid import UUID

import asyncpg

from ordering.models import KitchenTicket, Station


class TicketRepository:
    def __init__(self, conn: asyncpg.Connection) -> None:
        self._conn = conn

    async def insert_many(self, tickets: list[KitchenTicket]) -> None:
        await self._conn.executemany(
            """
            INSERT INTO kitchen_tickets (id, order_id, station, table_label, lines, allergen_alert, fired_at)
            VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)
            """,
            [
                (
                    t.id,
                    t.order_id,
                    t.station.value,
                    t.table,
                    json.dumps([line.model_dump() for line in t.lines]),
                    sorted(a.value for a in t.allergen_alert),
                    t.fired_at,
                )
                for t in tickets
            ],
        )

    async def open_for_station(self, location_id: UUID, station: Station) -> list[KitchenTicket]:
        rows = await self._conn.fetch(
            """
            SELECT t.* FROM kitchen_tickets t JOIN orders o ON o.id = t.order_id
            WHERE o.location_id = $1 AND t.station = $2 AND t.bumped_at IS NULL
            ORDER BY t.fired_at
            """,
            location_id,
            station.value,
        )
        return [
            KitchenTicket(
                id=r["id"],
                order_id=r["order_id"],
                station=Station(r["station"]),
                table=r["table_label"],
                lines=json.loads(r["lines"]),
                allergen_alert=frozenset(r["allergen_alert"]),
                fired_at=r["fired_at"],
                bumped_at=r["bumped_at"],
            )
            for r in rows
        ]

    async def bump(self, ticket_id: UUID, at: datetime) -> bool:
        result = await self._conn.execute(
            "UPDATE kitchen_tickets SET bumped_at = $2 WHERE id = $1 AND bumped_at IS NULL",
            ticket_id,
            at,
        )
        return result.endswith("1")
