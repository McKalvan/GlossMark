"""Order lifecycle: open, add lines, fire to the kitchen, serve, settle."""

from datetime import datetime
from decimal import Decimal
from uuid import UUID, uuid4

import httpx

from ordering.models import KitchenTicket, Order, OrderLine, OrderStatus
from ordering.models.allergen import AllergenDeclaration
from ordering.repositories.menu_repo import MenuRepository
from ordering.repositories.order_repo import OrderRepository
from ordering.repositories.ticket_repo import TicketRepository
from ordering.services.allergen_guard import enforce_allergen_rule
from ordering.services.ticket_router import route_ticket
from ordering.settings import get_settings


class InvalidTransition(Exception):
    pass


# @gloss implements .OrderFlow conf=0.95
class OrderFlow:
    def __init__(self, orders: OrderRepository, menu: MenuRepository, tickets: TicketRepository) -> None:
        self._orders = orders
        self._menu = menu
        self._tickets = tickets

    async def open_order(self, location_id: UUID, table: str, covers: int, lines: list[OrderLine],
                         allergens: AllergenDeclaration) -> Order:
        items = await self._menu.items_by_id([line.menu_item_id for line in lines])
        unavailable = [items[line.menu_item_id].name for line in lines if not items[line.menu_item_id].available]
        if unavailable:
            raise InvalidTransition(f"86'd: {', '.join(unavailable)}")
        order = Order(
            id=uuid4(),
            location_id=location_id,
            table=table,
            covers=covers,
            lines=lines,
            allergens=allergens,
            total=sum((items[line.menu_item_id].price * line.quantity for line in lines), Decimal("0.00")),
            opened_at=datetime.now().astimezone(),
        )
        await self._orders.save(order)
        return order

    async def fire(self, order_id: UUID) -> list[KitchenTicket]:
        order = await self._require(order_id, OrderStatus.OPEN)
        items = await self._menu.items_by_id([line.menu_item_id for line in order.lines])
        flagged = enforce_allergen_rule(order, items)
        order = order.model_copy(update={"status": OrderStatus.FIRED, "fired_at": datetime.now().astimezone()})
        tickets = route_ticket(order, items, flagged)
        await self._tickets.insert_many(tickets)
        await self._orders.save(order)
        await self._push_to_kds(tickets)
        return tickets

    async def mark_served(self, order_id: UUID) -> None:
        await self._require(order_id, OrderStatus.FIRED)
        await self._orders.set_status(order_id, OrderStatus.SERVED)

    async def void(self, order_id: UUID) -> None:
        order = await self._orders.get(order_id)
        if order is None or order.status is OrderStatus.PAID:
            raise InvalidTransition("cannot void a paid or unknown order")
        await self._orders.set_status(order_id, OrderStatus.VOID)

    async def _require(self, order_id: UUID, status: OrderStatus) -> Order:
        order = await self._orders.get(order_id)
        if order is None:
            raise KeyError(order_id)
        if order.status is not status:
            raise InvalidTransition(f"order {order_id} is {order.status}, expected {status}")
        return order

    async def _push_to_kds(self, tickets: list[KitchenTicket]) -> None:
        settings = get_settings()
        async with httpx.AsyncClient(timeout=settings.ticket_push_timeout_s) as client:
            for ticket in tickets:
                await client.post(settings.kds_push_url, json=ticket.model_dump(mode="json"))
