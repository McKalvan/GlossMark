import json
from uuid import UUID

import asyncpg

from ordering.models import Order, OrderStatus


class OrderRepository:
    def __init__(self, conn: asyncpg.Connection) -> None:
        self._conn = conn

    async def get(self, order_id: UUID) -> Order | None:
        row = await self._conn.fetchrow("SELECT document FROM orders WHERE id = $1", order_id)
        return Order.model_validate_json(row["document"]) if row else None

    async def save(self, order: Order) -> None:
        await self._conn.execute(
            """
            INSERT INTO orders (id, location_id, table_label, status, total, document, opened_at)
            VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
            ON CONFLICT (id) DO UPDATE
               SET status = excluded.status, total = excluded.total, document = excluded.document
            """,
            order.id,
            order.location_id,
            order.table,
            order.status.value,
            order.total,
            json.dumps(order.model_dump(mode="json")),
            order.opened_at,
        )

    async def set_status(self, order_id: UUID, status: OrderStatus) -> None:
        await self._conn.execute(
            "UPDATE orders SET status = $2, document = jsonb_set(document, '{status}', to_jsonb($2::text)) WHERE id = $1",
            order_id,
            status.value,
        )
