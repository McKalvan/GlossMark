---
name: allergen-audit
description: Audit the allergen rule end to end against docs/allergen-policy.md - the check, the per-station alert, the banner and their tests. Use before merging any change that touches orders, menu allergens or the kitchen display.
---

<!-- @gloss instructs dine:AllergenRule conf=0.9 scope=file -->

# Allergen audit

Work through the policy one rule at a time. For each rule, find the code that enforces it and the test that proves it. Report a table: rule, code, test, verdict.

1. **Declared before firing.** `AllergenDeclaration` is on `Order`; `OrderFlow.fire` calls `enforce_allergen_rule` before any ticket exists.
2. **Acknowledgement required.** `enforce_allergen_rule` raises `AllergenNotAcknowledged` when a conflict has no `acknowledged_by`; the fire route returns `422`.
3. **Per-station alert.** `route_ticket` sets `allergen_alert` only on stations that cook a flagged dish.
4. **May contain counts.** `find_conflicts` checks `may_contain`.
5. **Banner.** `renderAllergenBanner` pins every flagged open ticket.

Any rule without both code and a test is a finding. Do not fix it in the audit; open it as a separate change.
