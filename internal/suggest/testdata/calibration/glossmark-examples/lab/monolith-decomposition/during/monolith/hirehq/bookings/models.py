from django.db import models


# @gloss represents hire:Customer
class Customer(models.Model):
    name = models.CharField(max_length=200)
    account_number = models.CharField(max_length=20, unique=True)
    vat_registered = models.BooleanField(default=False)


# @gloss represents hire:Equipment
class Equipment(models.Model):
    sku = models.CharField(max_length=40, unique=True)
    description = models.CharField(max_length=200)
    day_rate_pence = models.PositiveIntegerField()


# @gloss represents hire:Hire
class Hire(models.Model):
    customer = models.ForeignKey(Customer, on_delete=models.PROTECT, related_name="hires")
    equipment = models.ForeignKey(Equipment, on_delete=models.PROTECT, related_name="hires")
    starts_on = models.DateField()
    ends_on = models.DateField()
    returned_on = models.DateField(null=True, blank=True)
