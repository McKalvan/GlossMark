from decimal import Decimal
from uuid import uuid4

import pytest

from ordering.models import Allergen, OrderLine, OrderStatus
from ordering.models.allergen import AllergenDeclaration
from ordering.services.allergen_guard import AllergenNotAcknowledged
from ordering.services.order_flow import InvalidTransition, OrderFlow
from tests.conftest import CANAL_STREET


class FakeOrders:
    def __init__(self):
        self.saved = {}

    async def get(self, order_id):
        return self.saved.get(order_id)

    async def save(self, order):
        self.saved[order.id] = order

    async def set_status(self, order_id, status):
        self.saved[order_id] = self.saved[order_id].model_copy(update={"status": status})


class FakeMenu:
    def __init__(self, menu):
        self.items = {i.id: i for i in menu.values()}

    async def items_by_id(self, ids):
        return {i: self.items[i] for i in ids}


class FakeTickets:
    def __init__(self):
        self.inserted = []

    async def insert_many(self, tickets):
        self.inserted.extend(tickets)


@pytest.fixture
def flow(menu, monkeypatch):
    f = OrderFlow(FakeOrders(), FakeMenu(menu), FakeTickets())
    monkeypatch.setattr(f, "_push_to_kds", lambda tickets: _noop())
    return f


async def _noop():
    return None


# @gloss verifies .OrderFlow conf=0.9
async def test_open_order_totals_the_lines(flow, menu):
    order = await flow.open_order(CANAL_STREET, "T4", 2, [OrderLine(menu_item_id=menu["satay"].id, quantity=2)],
                                  AllergenDeclaration())
    assert order.total == Decimal("17.00")


async def test_eighty_sixed_item_cannot_be_ordered(flow, menu):
    menu["squid"] = menu["squid"].model_copy(update={"available": False})
    flow._menu.items[menu["squid"].id] = menu["squid"]
    with pytest.raises(InvalidTransition):
        await flow.open_order(CANAL_STREET, "T4", 1, [OrderLine(menu_item_id=menu["squid"].id, quantity=1)],
                              AllergenDeclaration())


# @gloss? verifies .OrderFlow conf=0.75 id=order-fire-refusal-test note="proves fire leaves the order open when the allergen check refuses"
async def test_fire_refuses_unacknowledged_allergen(flow, menu):
    order = await flow.open_order(CANAL_STREET, "T9", 1, [OrderLine(menu_item_id=menu["satay"].id, quantity=1)],
                                  AllergenDeclaration(allergens=frozenset({Allergen.PEANUTS})))
    with pytest.raises(AllergenNotAcknowledged):
        await flow.fire(order.id)
    assert (await flow._orders.get(order.id)).status is OrderStatus.OPEN


async def test_cannot_serve_an_unknown_order(flow):
    with pytest.raises(KeyError):
        await flow.mark_served(uuid4())
