"""Hands a served order to the card processor and records the result.

Card data never touches this service: the terminal talks to the processor,
and we only create the intent and receive the signed webhook."""

import hashlib
import hmac
from decimal import Decimal
from uuid import UUID, uuid4

import httpx

from ordering.models import OrderStatus, PaymentIntent, PaymentStatus
from ordering.repositories.order_repo import OrderRepository
from ordering.settings import get_settings


class PaymentRejected(Exception):
    pass


# @gloss implements .PaymentHandoff conf=0.95
class PaymentHandoff:
    def __init__(self, orders: OrderRepository, client: httpx.AsyncClient | None = None) -> None:
        self._orders = orders
        self._client = client or httpx.AsyncClient(timeout=10.0)

    async def create_intent(self, order_id: UUID, tip: Decimal = Decimal("0.00")) -> PaymentIntent:
        order = await self._orders.get(order_id)
        if order is None or order.status is not OrderStatus.SERVED:
            raise PaymentRejected("only served orders can be paid")
        settings = get_settings()
        intent = PaymentIntent(id=uuid4(), order_id=order_id, amount=order.total + tip, tip=tip,
                               currency=settings.default_currency)
        resp = await self._client.post(
            f"{settings.payment_provider_url}/intents",
            headers={"Authorization": f"Bearer {settings.payment_provider_key}"},
            json={"reference": str(intent.id), "amount": str(intent.amount), "currency": intent.currency},
        )
        resp.raise_for_status()
        return intent.model_copy(update={"provider_ref": resp.json()["id"]})

    async def handle_webhook(self, body: bytes, signature: str) -> PaymentIntent | None:
        if not verify_signature(body, signature, get_settings().payment_webhook_secret):
            raise PaymentRejected("bad webhook signature")
        event = httpx.Response(200, content=body).json()
        if event["type"] != "intent.captured":
            return None
        order_id = UUID(event["data"]["metadata"]["order_id"])
        await self._orders.set_status(order_id, OrderStatus.PAID)
        return PaymentIntent(
            id=UUID(event["data"]["reference"]),
            order_id=order_id,
            amount=Decimal(event["data"]["amount"]),
            provider_ref=event["data"]["id"],
            status=PaymentStatus.CAPTURED,
        )


def verify_signature(body: bytes, signature: str, secret: str) -> bool:
    expected = hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)


# @gloss? implements pepper:NEW:SplitBill note="guests ask to split the bill; Checkout assumes one payment per order"
def split_evenly(total: Decimal, ways: int) -> list[Decimal]:
    """Split a bill into equal shares; the first share absorbs the rounding pennies."""
    if ways < 1:
        raise ValueError("ways must be at least 1")
    share = (total / ways).quantize(Decimal("0.01"))
    shares = [share] * ways
    shares[0] += total - share * ways
    return shares
