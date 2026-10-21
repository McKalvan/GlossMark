from django.db import models

from hirehq.bookings.models import Hire


# @gloss represents hire:Invoice
class Invoice(models.Model):
    hire = models.OneToOneField(Hire, on_delete=models.PROTECT, related_name="invoice")
    net_pence = models.PositiveIntegerField()
    vat_pence = models.PositiveIntegerField()
    issued_at = models.DateTimeField(auto_now_add=True)
