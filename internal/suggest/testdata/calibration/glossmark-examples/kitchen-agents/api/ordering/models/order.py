from datetime import datetime
from decimal import Decimal
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, Field

from ordering.models.allergen import AllergenDeclaration


class OrderStatus(StrEnum):
    OPEN = "open"
    FIRED = "fired"
    SERVED = "served"
    PAID = "paid"
    VOID = "void"


class OrderLine(BaseModel):
    menu_item_id: UUID
    quantity: int = Field(ge=1, le=50)
    modifiers: list[str] = Field(default_factory=list)
    seat: int | None = None


# @gloss represents dine:Order conf=0.95
class Order(BaseModel):
    id: UUID
    location_id: UUID
    table: str
    covers: int = Field(ge=1)
    lines: list[OrderLine]
    allergens: AllergenDeclaration = Field(default_factory=AllergenDeclaration)
    status: OrderStatus = OrderStatus.OPEN
    total: Decimal = Decimal("0.00")
    opened_at: datetime
    fired_at: datetime | None = None
