from uuid import UUID

import asyncpg
from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel, Field

from ordering.db import get_connection
from ordering.models import KitchenTicket, Order, OrderLine
from ordering.models.allergen import AllergenDeclaration
from ordering.repositories.menu_repo import MenuRepository
from ordering.repositories.order_repo import OrderRepository
from ordering.repositories.ticket_repo import TicketRepository
from ordering.services.allergen_guard import AllergenNotAcknowledged
from ordering.services.order_flow import InvalidTransition, OrderFlow

router = APIRouter()


class OpenOrderRequest(BaseModel):
    location_id: UUID
    table: str
    covers: int = Field(ge=1)
    lines: list[OrderLine]
    allergens: AllergenDeclaration = Field(default_factory=AllergenDeclaration)


def order_flow(conn: asyncpg.Connection = Depends(get_connection)) -> OrderFlow:
    return OrderFlow(OrderRepository(conn), MenuRepository(conn), TicketRepository(conn))


@router.post("", response_model=Order, status_code=status.HTTP_201_CREATED)
async def open_order(req: OpenOrderRequest, flow: OrderFlow = Depends(order_flow)) -> Order:
    try:
        return await flow.open_order(req.location_id, req.table, req.covers, req.lines, req.allergens)
    except InvalidTransition as exc:
        raise HTTPException(status.HTTP_409_CONFLICT, str(exc)) from exc


@router.post("/{order_id}/fire", response_model=list[KitchenTicket])
async def fire_order(order_id: UUID, flow: OrderFlow = Depends(order_flow)) -> list[KitchenTicket]:
    try:
        return await flow.fire(order_id)
    except AllergenNotAcknowledged as exc:
        raise HTTPException(
            status.HTTP_422_UNPROCESSABLE_ENTITY,
            {"message": str(exc), "conflicts": [c.item_name for c in exc.conflicts]},
        ) from exc
    except InvalidTransition as exc:
        raise HTTPException(status.HTTP_409_CONFLICT, str(exc)) from exc


@router.post("/{order_id}/served", status_code=status.HTTP_204_NO_CONTENT)
async def mark_served(order_id: UUID, flow: OrderFlow = Depends(order_flow)) -> None:
    await flow.mark_served(order_id)


@router.post("/{order_id}/void", status_code=status.HTTP_204_NO_CONTENT)
async def void_order(order_id: UUID, flow: OrderFlow = Depends(order_flow)) -> None:
    try:
        await flow.void(order_id)
    except InvalidTransition as exc:
        raise HTTPException(status.HTTP_409_CONFLICT, str(exc)) from exc
