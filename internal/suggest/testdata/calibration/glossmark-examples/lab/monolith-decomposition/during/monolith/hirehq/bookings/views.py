from django.db import transaction
from django.http import JsonResponse
from django.views.decorators.http import require_POST

from .models import Customer, Equipment, Hire


# @gloss implements .BookingView
@require_POST
@transaction.atomic
def create_hire(request):
    customer = Customer.objects.get(account_number=request.POST["account"])
    equipment = Equipment.objects.select_for_update().get(sku=request.POST["sku"])
    starts_on, ends_on = request.POST["starts_on"], request.POST["ends_on"]
    clash = equipment.hires.filter(starts_on__lte=ends_on, ends_on__gte=starts_on, returned_on__isnull=True)
    if clash.exists():
        return JsonResponse({"error": "equipment already on hire"}, status=409)
    hire = Hire.objects.create(customer=customer, equipment=equipment, starts_on=starts_on, ends_on=ends_on)
    return JsonResponse({"hire": hire.pk}, status=201)
