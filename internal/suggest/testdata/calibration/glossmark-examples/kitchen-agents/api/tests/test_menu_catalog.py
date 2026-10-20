from ordering.services.menu_catalog import group_into_sections


# @gloss verifies .MenuCatalog conf=0.9
def test_sections_follow_service_order(menu):
    for name, section in [("satay", "Small plates"), ("brownie", "Desserts"), ("slaw", "Sides")]:
        menu[name] = menu[name].model_copy(update={"section": section})
    sections = group_into_sections(list(menu.values()))
    assert [s.name for s in sections] == ["Small plates", "Sides", "Desserts"]


# @gloss? verifies .MenuCatalog conf=0.5 id=menu-unknown-section-test note="checks sort order only, not publishing"
def test_unknown_sections_sort_last(menu):
    special = menu["squid"].model_copy(update={"section": "Chef's table"})
    sections = group_into_sections([special, menu["satay"]])
    assert sections[-1].name == "Chef's table"
