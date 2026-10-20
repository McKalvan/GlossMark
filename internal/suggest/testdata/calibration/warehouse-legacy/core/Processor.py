from db import q


# @gloss implements wh:ReturnsProcessing
class Processor:
    """Works through the queue."""

    def load(self, rma_id):
        return q("select * from tbl_rma where id = %s", rma_id)

    def vendor(self, rma_id):
        return q("select * from tbl_rma_vendor where rma = %s", rma_id)

    # @gloss implements .ReturnGrader
    def grade(self, item):
        """Grade a returned item A, B or scrap."""
        return "A"
