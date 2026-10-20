<!-- @gloss documents shc:LabelSpec conf=0.9 -->
# Shipping label format

Every shipment gets one shipping label, rendered on first request to `GET /shipments/{id}/label` and
stored in `shipping_labels`. The response is `x-application/zpl`, ready to send to a thermal printer.

| Property | Value |
| --- | --- |
| Language | ZPL II |
| Stock | 4 x 6 in (812 x 1218 dots) |
| Resolution | 203 dpi |
| Barcode | Code 128 of the tracking number, module width 3 dots, height 160 dots, human-readable line below |

The constants live in `LabelSpec`; `LabelRenderer` lays out the label.

## Layout (top to bottom)

1. Brand line with service level (`STANDARD` / `EXPRESS`) and carrier code
2. Sender address on one line
3. Delivery address, postcode printed largest for sortation
4. The promised delivery window in local time, e.g. `Deliver Thu 22 Oct 10:00-12:00`
5. Parcel count and total weight
6. Code 128 barcode of the tracking number

## Tracking numbers

`KF` followed by 10 digits and a Luhn check digit, e.g. `KF12345678903`. The barcode uses ZPL's automatic
subset selection, so the prefix is encoded in subset B and the digits in subset C.

Labels are not regenerated after booking. Cancelled shipments return 404.
