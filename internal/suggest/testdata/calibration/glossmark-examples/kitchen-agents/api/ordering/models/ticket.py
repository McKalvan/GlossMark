from datetime import datetime
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, Field

from ordering.models.allergen import Allergen


# @gloss represents dine:Station conf=0.95
class Station(StrEnum):
    GRILL = "grill"
    FRY = "fry"
    SAUTE = "saute"
    COLD = "cold"
    PASTRY = "pastry"
    PASS = "pass"


class TicketLine(BaseModel):
    name: str
    quantity: int
    modifiers: list[str] = Field(default_factory=list)
    seat: int | None = None


# @gloss represents dine:KitchenTicket conf=0.95
class KitchenTicket(BaseModel):
    id: UUID
    order_id: UUID
    station: Station
    table: str
    lines: list[TicketLine]
    allergen_alert: frozenset[Allergen] = Field(default_factory=frozenset)
    fired_at: datetime
    bumped_at: datetime | None = None
