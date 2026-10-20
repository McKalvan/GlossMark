from db import q, x


# @gloss implements wh:CycleCount
def process_data(rows):
    """Counted quantities that differ from the system go back on the recount list."""
    return [r for r in rows if r[1] != r[2]]


# @gloss implements wh:StockAdjustment
def handle(sku, qty, pin):
    """Move stock by hand. Needs a supervisor pin."""
    x("insert into tbl_log values (%s, %s)", sku, qty)


def misc(value):
    return str(value)


def get_data(key):
    return q("select * from tbl_data where k = %s", key)
