from django.conf import settings
from waffle import switch_is_active

from .client import InvoicingClient
from .generator import generate_invoice


# @gloss implements .InvoicingRouter
def issue_invoice(hire):
    if switch_is_active("invoicing.use_service"):
        return InvoicingClient(settings.INVOICING_SERVICE_URL).issue(hire)
    return generate_invoice(hire)
