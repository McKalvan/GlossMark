<!-- @gloss documents circ:SelfCheckApi scope=file conf=0.85 -->
# Self-checkout kiosks

Added in 2022 when the Main and Eastside branches got ShelfServe SC-400 kiosks.

The kiosks speak gRPC ([selfcheck.proto](proto/riverbend/selfcheck/v1/selfcheck.proto)) to a small gateway that runs next to the CIRC web app and calls the same `CirculationService` and `RenewalService` the desk uses, so a kiosk can never lend what the desk would refuse.

| Kiosk call | CIRC rule it reuses |
| --- | --- |
| `Checkout` | patron block, borrowing limit, item circulates |
| `Renew` | renewal limit, nobody waiting |

Kiosks do not check items in. Returns go in the book drop and are checked in by staff, so holds are trapped at the desk.

Kiosk ids, branches and idle timeouts are in `config/kiosk.json`.
