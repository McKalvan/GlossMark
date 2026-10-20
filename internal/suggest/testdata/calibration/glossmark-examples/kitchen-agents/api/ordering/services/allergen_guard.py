"""The allergen rule: no line containing a guest's declared allergen reaches a station
unless a member of staff has acknowledged it, and every ticket carrying it is flagged."""

from dataclasses import dataclass

from ordering.models import Allergen, MenuItem, Order


@dataclass(frozen=True)
class AllergenConflict:
    menu_item_id: str
    item_name: str
    allergens: frozenset[Allergen]
    may_contain_only: bool


class AllergenNotAcknowledged(Exception):
    def __init__(self, conflicts: list[AllergenConflict]) -> None:
        names = ", ".join(c.item_name for c in conflicts)
        super().__init__(f"declared allergens present in: {names}; staff acknowledgement required")
        self.conflicts = conflicts


def find_conflicts(order: Order, items: dict) -> list[AllergenConflict]:
    declared = order.allergens.allergens
    if not declared:
        return []
    conflicts: list[AllergenConflict] = []
    for line in order.lines:
        item: MenuItem = items[line.menu_item_id]
        hits = item.allergens & declared
        traces = item.may_contain & declared
        if hits or traces:
            conflicts.append(
                AllergenConflict(
                    menu_item_id=str(item.id),
                    item_name=item.name,
                    allergens=hits | traces,
                    may_contain_only=not hits,
                )
            )
    return conflicts


# @gloss implements .AllergenGuard conf=0.9
def enforce_allergen_rule(order: Order, items: dict) -> frozenset[Allergen]:
    """Raise unless every conflict is acknowledged; return the allergens tickets must flag."""
    conflicts = find_conflicts(order, items)
    if conflicts and not order.allergens.acknowledged_by:
        raise AllergenNotAcknowledged(conflicts)
    flagged: frozenset[Allergen] = frozenset()
    for c in conflicts:
        flagged |= c.allergens
    return flagged
