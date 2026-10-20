---
name: add-menu-item
description: Add a dish to a Pepper Row location's menu end to end - migration seed, allergen rows, section placement and a test. Use when asked to add, price or place a new menu item.
---

<!-- @gloss instructs pepper:MenuCatalog conf=0.9 scope=file -->

# Add a menu item

1. Ask for: location, section, name, price, station, and the allergen list the head chef signed off (contains and may-contain). Do not guess allergens.
2. Add the item to the location's seed migration, or write a new migration if the seed has shipped. Migrations are append only.
3. Hand the allergen rows to the `allergen-auditor` subagent; it writes `menu_item_allergens`.
4. If the section is new, add it to `SECTION_ORDER` in `api/ordering/services/menu_catalog.py`.
5. Add a case to `api/tests/test_menu_catalog.py` that shows the item in its section.
6. `make test-api`.
