"""Builds the menu a location publishes to servers and the online ordering page."""

from itertools import groupby
from uuid import UUID

from ordering.models import MenuItem, MenuSection
from ordering.repositories.menu_repo import MenuRepository

SECTION_ORDER = ("Snacks", "Small plates", "Large plates", "Sides", "Desserts", "Drinks")


# @gloss implements .MenuCatalog conf=0.95
class MenuCatalog:
    def __init__(self, repo: MenuRepository) -> None:
        self._repo = repo

    async def published_menu(self, location_id: UUID) -> list[MenuSection]:
        items = await self._repo.items_for_location(location_id)
        return group_into_sections(items)

    # @gloss? implements pepper:MenuAvailability note="86ing a dish is its own mechanism; MenuCatalog only publishes"
    async def mark_unavailable(self, item_id: UUID) -> None:
        await self._repo.set_available(item_id, False)

    async def restore(self, item_id: UUID) -> None:
        await self._repo.set_available(item_id, True)


def group_into_sections(items: list[MenuItem]) -> list[MenuSection]:
    def rank(section: str) -> int:
        return SECTION_ORDER.index(section) if section in SECTION_ORDER else len(SECTION_ORDER)

    ordered = sorted(items, key=lambda i: (rank(i.section), i.name))
    return [
        MenuSection(name=name, position=rank(name), items=list(group))
        for name, group in groupby(ordered, key=lambda i: i.section)
    ]
