"""Returns: an item comes back over the desk or through the drop box."""


# @gloss implements .ReturnPipeline
def check_in(barcodes):
    for barcode in barcodes:
        yield close_loan(barcode)


# @gloss? implements circ:NEW:DamageCheck note="inspecting returned items has no concept yet"
def inspect(item):
    item.needs_repair = item.pages_missing > 0


# @gloss implements .ReturnNotifier
def announce_return(barcode):
    publish("item-returned", barcode)
