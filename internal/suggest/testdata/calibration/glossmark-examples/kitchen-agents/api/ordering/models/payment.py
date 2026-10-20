from decimal import Decimal
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel


class PaymentStatus(StrEnum):
    PENDING = "pending"
    AUTHORISED = "authorised"
    CAPTURED = "captured"
    FAILED = "failed"


# @gloss represents dine:Payment conf=0.45
class PaymentIntent(BaseModel):
    id: UUID
    order_id: UUID
    amount: Decimal
    tip: Decimal = Decimal("0.00")
    currency: str = "GBP"
    provider_ref: str | None = None
    status: PaymentStatus = PaymentStatus.PENDING
