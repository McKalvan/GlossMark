import pytest

from ordering.models import Allergen
from ordering.services.allergen_guard import AllergenNotAcknowledged, enforce_allergen_rule, find_conflicts


def test_no_declaration_means_no_conflict(make_order):
    order, items = make_order("satay", "squid")
    assert find_conflicts(order, items) == []
    assert enforce_allergen_rule(order, items) == frozenset()


# @gloss verifies .AllergenGuard conf=0.9
def test_declared_allergen_blocks_firing_without_acknowledgement(make_order):
    order, items = make_order("satay", "slaw", allergens=[Allergen.PEANUTS])
    with pytest.raises(AllergenNotAcknowledged) as exc:
        enforce_allergen_rule(order, items)
    assert [c.item_name for c in exc.value.conflicts] == ["Chicken satay"]


def test_acknowledged_allergen_is_flagged_for_the_kitchen(make_order):
    order, items = make_order("satay", allergens=[Allergen.PEANUTS], acknowledged_by="server:amara")
    assert enforce_allergen_rule(order, items) == frozenset({Allergen.PEANUTS})


# @gloss? verifies dine:AllergenRule conf=0.92 id=allergen-may-contain-test note="rule 4 of the policy: a trace is a conflict"
def test_may_contain_counts_as_a_conflict(make_order):
    order, items = make_order("brownie", allergens=[Allergen.TREE_NUTS])
    conflicts = find_conflicts(order, items)
    assert len(conflicts) == 1
    assert conflicts[0].may_contain_only is True
