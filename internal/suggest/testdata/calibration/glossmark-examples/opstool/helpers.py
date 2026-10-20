from utils.common import safe_div, to_int
from utils.db import get_stuff, save_stuff

# @gloss publishes-value wh:CountTolerance scope=line conf=0.9
TOL = 0.02


# @gloss implements .CountReconciler conf=0.9
def process_data(rows, cfg):
    """Compare what the counters found in each bin with what the system says
    is there. Anything off by more than TOL goes back on the recount list."""
    out = []
    for r in rows:
        bin_id, sku = r["loc"], r["item"]
        found = to_int(r["qty"])
        have = get_stuff("SELECT qty FROM tbl_data WHERE loc=%s AND item=%s", (bin_id, sku))
        have = have[0]["qty"] if have else 0
        d = calc(found, have)
        save_stuff(
            "INSERT INTO tbl_cnt (loc, item, found, sys_qty, diff) VALUES (%s,%s,%s,%s,%s)",
            (bin_id, sku, found, have, d),
        )
        if chk(d):
            out.append({"loc": bin_id, "item": sku, "found": found, "have": have, "diff": d})
    return out


# @gloss implements wh:CountVariance conf=0.8
def calc(a, b):
    return safe_div(a - b, b if b else 1)


# @gloss implements wh:CountTolerance conf=0.8
def chk(d):
    return abs(d) > TOL


# @gloss implements wh:StockAdjustment conf=0.85
def handle(data, cfg):
    """Move stock by hand. Needs a supervisor pin over the limit."""
    qty = to_int(data.get("qty"))
    if abs(qty) > cfg.getint("main", "adj_limit") and data.get("pin") != cfg.get("main", "sup_pin"):
        return {"ok": False, "err": "pin"}
    save_stuff(
        "UPDATE tbl_data SET qty = qty + %s WHERE loc=%s AND item=%s",
        (qty, data["loc"], data["item"]),
    )
    save_stuff(
        "INSERT INTO tbl_log (kind, loc, item, qty, who) VALUES ('ADJ',%s,%s,%s,%s)",
        (data["loc"], data["item"], qty, data.get("user")),
    )
    return {"ok": True}


def fmt_row(r):
    return "{loc:<10}{item:<14}{found:>6}{have:>6}{diff:>8.2%}".format(**r)
