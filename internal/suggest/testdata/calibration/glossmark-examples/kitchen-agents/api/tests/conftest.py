from datetime import datetime, timezone
from decimal import Decimal
from uuid import UUID, uuid4

import pytest

from ordering.models import Allergen, MenuItem, Order, OrderLine
from ordering.models.allergen import AllergenDeclaration

CANAL_STREET = UUID("6f1d2c3e-0000-4000-8000-000000000001")


def item(name: str, station: str, price: str, contains=(), may_contain=(), available=True) -> MenuItem:
    return MenuItem(
        id=uuid4(),
        location_id=CANAL_STREET,
        section="Small plates",
        name=name,
        price=Decimal(price),
        allergens=frozenset(contains),
        may_contain=frozenset(may_contain),
        station=station,
        available=available,
    )


@pytest.fixture
def menu() -> dict[str, MenuItem]:
    return {
        "satay": item("Chicken satay", "grill", "8.50", contains=[Allergen.PEANUTS, Allergen.SOYA]),
        "squid": item("Salt and pepper squid", "fry", "9.00", contains=[Allergen.MOLLUSCS, Allergen.GLUTEN]),
        "slaw": item("Fennel slaw", "cold", "4.50", contains=[Allergen.MUSTARD, Allergen.CELERY]),
        "brownie": item("Brownie", "pastry", "6.00", contains=[Allergen.EGGS, Allergen.MILK],
                        may_contain=[Allergen.TREE_NUTS]),
    }


@pytest.fixture
def make_order(menu):
    def _make(*names: str, allergens=(), acknowledged_by=None) -> tuple[Order, dict]:
        lines = [OrderLine(menu_item_id=menu[n].id, quantity=1) for n in names]
        order = Order(
            id=uuid4(),
            location_id=CANAL_STREET,
            table="T12",
            covers=2,
            lines=lines,
            allergens=AllergenDeclaration(allergens=frozenset(allergens), acknowledged_by=acknowledged_by),
            opened_at=datetime(2026, 10, 19, 19, 30, tzinfo=timezone.utc),
        )
        return order, {menu[n].id: menu[n] for n in names}

    return _make
