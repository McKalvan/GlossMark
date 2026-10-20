from utils.db import get_stuff, save_stuff
from utils.misc import log_it


# @gloss implements .ReturnGrader conf=0.9
class Processor:
    """Does the stuff that comes back."""

    def __init__(self, cfg):
        self.cfg = cfg

    def process(self, record):
        if not self.validate(record):
            return {"ok": False}
        out = []
        for ln in record["lines"]:
            g = self.grade(ln)
            if g == "A":
                save_stuff("UPDATE tbl_data SET qty = qty + %s WHERE loc = %s AND item = %s",
                           (ln["qty"], self.cfg.get("main", "rtn_loc"), ln["item"]))
            elif g == "V":
                save_stuff("INSERT INTO tbl_rma_vendor (rma_no, item, qty) VALUES (%s,%s,%s)",
                           (record["rma_no"], ln["item"], ln["qty"]))
            out.append({"item": ln["item"], "grade": g})
        self.finish(record, out)
        return {"ok": True, "lines": out}

    def validate(self, record):
        r = get_stuff("SELECT status FROM tbl_rma WHERE rma_no = %s", (record.get("rma_no"),))
        return bool(r) and r[0]["status"] == "OPEN"

    def grade(self, ln):
        if ln.get("damaged"):
            return "V" if ln.get("vendor_ok") else "X"
        return "A" if ln.get("sealed") else "B"

    def finish(self, record, out):
        save_stuff("UPDATE tbl_rma SET status = 'DONE' WHERE rma_no = %s", (record["rma_no"],))
        log_it("rma", record["rma_no"], len(out))
