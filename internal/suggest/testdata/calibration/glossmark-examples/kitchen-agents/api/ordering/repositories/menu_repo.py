from uuid import UUID

import asyncpg

from ordering.models import Allergen, MenuItem


def _row_to_item(row: asyncpg.Record) -> MenuItem:
    return MenuItem(
        id=row["id"],
        location_id=row["location_id"],
        section=row["section"],
        name=row["name"],
        description=row["description"],
        price=row["price"],
        allergens=frozenset(Allergen(a) for a in row["allergens"]),
        may_contain=frozenset(Allergen(a) for a in row["may_contain"]),
        station=row["station"],
        available=row["available"],
    )


# @gloss? implements .MenuCatalog conf=0.62 id=menu-repo-helper note="the repository loads every menu"
class MenuRepository:
    def __init__(self, conn: asyncpg.Connection) -> None:
        self._conn = conn

    async def items_for_location(self, location_id: UUID) -> list[MenuItem]:
        rows = await self._conn.fetch(
            """
            SELECT i.*, coalesce(array_agg(a.allergen) FILTER (WHERE a.kind = 'contains'), '{}') AS allergens,
                   coalesce(array_agg(a.allergen) FILTER (WHERE a.kind = 'may_contain'), '{}') AS may_contain
            FROM menu_items i
            LEFT JOIN menu_item_allergens a ON a.menu_item_id = i.id
            WHERE i.location_id = $1 AND i.retired_at IS NULL
            GROUP BY i.id
            ORDER BY i.section, i.position
            """,
            location_id,
        )
        return [_row_to_item(r) for r in rows]

    async def items_by_id(self, ids: list[UUID]) -> dict[UUID, MenuItem]:
        rows = await self._conn.fetch(
            """
            SELECT i.*, coalesce(array_agg(a.allergen) FILTER (WHERE a.kind = 'contains'), '{}') AS allergens,
                   coalesce(array_agg(a.allergen) FILTER (WHERE a.kind = 'may_contain'), '{}') AS may_contain
            FROM menu_items i
            LEFT JOIN menu_item_allergens a ON a.menu_item_id = i.id
            WHERE i.id = ANY($1::uuid[])
            GROUP BY i.id
            """,
            ids,
        )
        return {r["id"]: _row_to_item(r) for r in rows}

    async def set_available(self, item_id: UUID, available: bool) -> None:
        await self._conn.execute(
            "UPDATE menu_items SET available = $2, updated_at = now() WHERE id = $1",
            item_id,
            available,
        )
