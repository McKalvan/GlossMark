from decimal import Decimal
from uuid import UUID

from pydantic import BaseModel, Field

from ordering.models.allergen import Allergen


# @gloss represents dine:MenuItem conf=0.95
class MenuItem(BaseModel):
    id: UUID
    location_id: UUID
    section: str
    name: str = Field(max_length=80)
    description: str = ""
    price: Decimal = Field(gt=0, decimal_places=2)
    allergens: frozenset[Allergen] = Field(default_factory=frozenset)
    may_contain: frozenset[Allergen] = Field(default_factory=frozenset)
    station: str
    available: bool = True


class MenuSection(BaseModel):
    name: str
    position: int
    items: list[MenuItem]
