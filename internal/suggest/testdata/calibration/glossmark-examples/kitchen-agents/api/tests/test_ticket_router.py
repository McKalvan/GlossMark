from datetime import datetime, timezone

from ordering.models import Allergen, Station
from ordering.services.ticket_router import route_ticket


# @gloss verifies .TicketRouter conf=0.9
def test_one_ticket_per_station(make_order):
    order, items = make_order("satay", "squid", "slaw")
    order = order.model_copy(update={"fired_at": datetime(2026, 10, 19, 19, 45, tzinfo=timezone.utc)})
    tickets = route_ticket(order, items, frozenset())
    assert sorted(t.station for t in tickets) == [Station.COLD, Station.FRY, Station.GRILL]
    assert all(t.fired_at == order.fired_at for t in tickets)


# @gloss? verifies .AllergenGuard conf=0.6 id=allergen-station-alert-test note="rule 3: the router computes the alert, the guard decides it"
def test_allergen_alert_only_on_the_station_that_cooks_it(make_order):
    order, items = make_order("satay", "slaw", allergens=[Allergen.PEANUTS], acknowledged_by="server:amara")
    tickets = {t.station: t for t in route_ticket(order, items, frozenset({Allergen.PEANUTS}))}
    assert tickets[Station.GRILL].allergen_alert == frozenset({Allergen.PEANUTS})
    assert tickets[Station.COLD].allergen_alert == frozenset()
