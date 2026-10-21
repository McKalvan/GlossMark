import datetime

import pytest

from hirehq.invoicing.generator import generate_invoice


# @gloss verifies hire:Invoicing
@pytest.mark.django_db
class TestGenerateInvoice:
    def test_prices_inclusive_days(self, returned_hire):
        returned_hire.returned_on = returned_hire.starts_on + datetime.timedelta(days=2)
        invoice = generate_invoice(returned_hire)
        assert invoice.net_pence == 3 * returned_hire.equipment.day_rate_pence

    def test_refuses_open_hire(self, open_hire):
        with pytest.raises(ValueError):
            generate_invoice(open_hire)
