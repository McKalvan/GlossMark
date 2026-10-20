from collections import defaultdict

from utils.common import chunk, now_str
from utils.db import get_stuff, save_stuff
from utils.misc import log_it, send_mail

# classes that may not go on the same cart (see the binder in the office)
BAD = {("3", "5.1"), ("5.1", "3"), ("8", "4.3"), ("4.3", "8")}


class Manager:
    def __init__(self, cfg):
        self.cfg = cfg
        self.max_lines = cfg.getint("main", "cart_lines", fallback=40)

    # @gloss implements .ReplenishmentRun conf=0.9
    def doIt(self):
        """Top up the pick faces from the upstairs racks."""
        low = get_stuff(
            "SELECT l.loc, d.item, d.qty, l.min_qty, l.max_qty FROM tbl_loc l "
            "JOIN tbl_data d ON d.loc = l.loc WHERE l.zone = 'P' AND d.qty < l.min_qty"
        )
        n = 0
        for r in low:
            src = self._get(r["item"])
            if not src:
                send_mail(self.cfg.get("main", "sup_mail"), "nothing upstairs", r["item"])
                continue
            want = r["max_qty"] - r["qty"]
            self._put(src["loc"], r["loc"], r["item"], min(want, src["qty"]), "R")
            n += 1
        log_it("doIt", n)
        return {"moves": n}

    # @gloss implements .CartBuilder conf=0.9
    def run_batch(self, cutoff):
        """Group today's orders into carts for the given truck time."""
        lines = get_stuff(
            "SELECT o.ord_no, o.line_no, o.item, o.qty, m.cls, d.loc FROM tbl_ord o "
            "JOIN tbl_master m ON m.item = o.item JOIN tbl_data d ON d.item = o.item "
            "WHERE o.status = 'N' AND o.cutoff = %s ORDER BY d.loc",
            (cutoff,),
        )
        by_ord = defaultdict(list)
        for ln in lines:
            by_ord[ln["ord_no"]].append(ln)
        carts, cur = [], []
        for ord_no, ls in by_ord.items():
            if len(cur) + len(ls) > self.max_lines or not self._ok(cur, ls):
                carts.append(cur)
                cur = []
            cur.extend(ls)
        if cur:
            carts.append(cur)
        for i, cart in enumerate(carts, 1):
            for part in chunk(cart, 20):
                for ln in part:
                    save_stuff(
                        "INSERT INTO tbl_queue (batch, ord_no, line_no, loc, item, qty, kind, ts) "
                        "VALUES (%s,%s,%s,%s,%s,%s,'P',%s)",
                        (f"{cutoff}-{i}", ln["ord_no"], ln["line_no"], ln["loc"], ln["item"], ln["qty"], now_str()),
                    )
        return {"batches": len(carts)}

    def handle_request(self, kind, data):
        if kind == "repl":
            return self.doIt()
        if kind == "batch":
            return self.run_batch(data["cutoff"])
        return self.process(data)

    def process(self, data):
        rows = get_stuff("SELECT * FROM tbl_queue WHERE batch = %s", (data["batch"],))
        for r in rows:
            save_stuff("UPDATE tbl_queue SET done = 1 WHERE id = %s", (r["id"],))
        return {"done": len(rows)}

    def _get(self, item):
        r = get_stuff(
            "SELECT d.loc, d.qty FROM tbl_data d JOIN tbl_loc l ON l.loc = d.loc "
            "WHERE l.zone = 'R' AND d.item = %s AND d.qty > 0 ORDER BY d.qty DESC LIMIT 1",
            (item,),
        )
        return r[0] if r else None

    def _put(self, frm, to, item, qty, kind):
        save_stuff(
            "INSERT INTO tbl_queue (loc_from, loc, item, qty, kind, ts) VALUES (%s,%s,%s,%s,%s,%s)",
            (frm, to, item, qty, kind, now_str()),
        )

    # @gloss implements wh:HazmatHandling conf=0.85
    def _ok(self, cur, ls):
        have = {x["cls"] for x in cur if x["cls"]}
        new = {x["cls"] for x in ls if x["cls"]}
        return not any((a, b) in BAD for a in have for b in new)
