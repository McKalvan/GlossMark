from db import q, x


class Manager:
    """Does everything for the warehouse."""

    # @gloss implements .ReplenishmentRun
    def doIt(self, zone):
        """Top up the pick faces from the upstairs racks."""
        for row in q("select * from tbl_loc where zone = %s", zone):
            x("update tbl_loc set qty = %s", row)

    # @gloss implements .WaveBatch
    def run_batch(self, cutoff):
        """Build the pick waves for every open order before the cutoff."""
        orders = q("select * from tbl_ord where status = 'open'")
        return [orders]

    # @gloss implements .HazmatRule
    def _ok(self, sku, loc):
        return sku not in loc

    def save(self, row):
        x("insert into tbl_data values (%s)", row)

    def get(self, key):
        return q("select * from tbl_data where k = %s", key)
