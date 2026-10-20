# Allergen policy

Pepper Row serves guests with severe allergies every night. This is the rule the ordering system enforces, written the way the head chefs agreed it.

<!-- @gloss documents dine:AllergenRule conf=0.9 -->
## The rule

1. A server records every allergen a guest declares on the order before it is fired.
2. If any line contains a declared allergen, or may contain one, the order cannot be fired until a member of staff acknowledges it on the till. The API refuses with `422` and names the dishes.
3. Every kitchen ticket for a station that cooks a flagged dish carries an allergen alert. The kitchen display pins it to the top of that station's screen until the ticket is bumped.
4. "May contain" counts. A trace is a conflict.

## Who changes what

Allergen data for a dish (`menu_item_allergens`) is recorded by the head chef of the location and nobody else. The allergen-auditor agent may propose changes to the rule's code and tests; a human reviews every one, because a mistake here can put a guest in hospital.

## Where it lives

- `api/ordering/services/allergen_guard.py`: the check.
- `api/ordering/services/ticket_router.py`: the per-station alert.
- `kds/src/views/AllergenBanner.ts`: the banner.
