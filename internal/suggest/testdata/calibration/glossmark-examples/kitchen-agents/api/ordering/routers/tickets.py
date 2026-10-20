from datetime import datetime
from uuid import UUID

import asyncpg
from fastapi import APIRouter, Depends, HTTPException, status

from ordering.db import get_connection
from ordering.models import KitchenTicket, Station
from ordering.repositories.ticket_repo import TicketRepository

router = APIRouter()


def tickets(conn: asyncpg.Connection = Depends(get_connection)) -> TicketRepository:
    return TicketRepository(conn)


@router.get("/{location_id}/{station}", response_model=list[KitchenTicket])
async def open_tickets(location_id: UUID, station: Station,
                       repo: TicketRepository = Depends(tickets)) -> list[KitchenTicket]:
    return await repo.open_for_station(location_id, station)


# @gloss? implements .TicketRouter conf=0.45 id=ticket-bump-route note="bumping may belong to the display, not the router"
@router.post("/{ticket_id}/bump", status_code=status.HTTP_204_NO_CONTENT)
async def bump_ticket(ticket_id: UUID, repo: TicketRepository = Depends(tickets)) -> None:
    if not await repo.bump(ticket_id, datetime.now().astimezone()):
        raise HTTPException(status.HTTP_404_NOT_FOUND, "ticket not found or already bumped")
