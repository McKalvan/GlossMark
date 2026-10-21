from decimal import Decimal, ROUND_HALF_UP

from django.db import transaction

from .models import Invoice

VAT_RATE = Decimal("0.20")


# @gloss implements .InvoiceGenerator
@transaction.atomic
def generate_invoice(hire):
    if hire.returned_on is None:
        raise ValueError(f"hire {hire.pk} has not been returned")
    days = (hire.returned_on - hire.starts_on).days + 1
    net = hire.equipment.day_rate_pence * days
    vat = int((Decimal(net) * VAT_RATE).quantize(Decimal("1"), rounding=ROUND_HALF_UP)) if hire.customer.vat_registered else 0
    return Invoice.objects.create(hire=hire, net_pence=net, vat_pence=vat)
