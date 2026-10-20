<!-- @gloss documents dine:Ordering conf=0.9 -->
# Order lifecycle

An order moves through five states. Only the transitions below exist; anything else is a `409`.

| From | To | Who | Endpoint |
| --- | --- | --- | --- |
| (none) | open | server, online ordering | `POST /orders` |
| open | fired | server | `POST /orders/{id}/fire` |
| fired | served | expo at the pass | `POST /orders/{id}/served` |
| served | paid | card processor webhook | `POST /payments/webhook` |
| open, fired, served | void | manager | `POST /orders/{id}/void` |

## Firing

Firing runs the allergen check, splits the order into one ticket per station, stores the tickets and pushes each to the kitchen display. A station sees its ticket within `ORDERING_TICKET_PUSH_TIMEOUT_S` seconds or the push fails loudly.

<!-- @gloss documents dine:Checkout conf=0.9 -->
## Paying

The API never sees card data. It creates a payment intent with the processor (Tillpoint) for the order total plus tip, and the processor calls the webhook when the terminal captures it. A bad signature is a `400`.

## 86'd items

When the kitchen runs out of a dish, a manager marks it unavailable (`POST /locations/{id}/menu/items/{item}/86`). New orders containing it are refused until it is restored.
