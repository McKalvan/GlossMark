from decimal import Decimal
from uuid import UUID

import asyncpg
from fastapi import APIRouter, Depends, Header, HTTPException, Request, status
from pydantic import BaseModel

from ordering.db import get_connection
from ordering.models import PaymentIntent
from ordering.repositories.order_repo import OrderRepository
from ordering.services.payment_handoff import PaymentHandoff, PaymentRejected

router = APIRouter()


class IntentRequest(BaseModel):
    order_id: UUID
    tip: Decimal = Decimal("0.00")


def handoff(conn: asyncpg.Connection = Depends(get_connection)) -> PaymentHandoff:
    return PaymentHandoff(OrderRepository(conn))


@router.post("/intents", response_model=PaymentIntent, status_code=status.HTTP_201_CREATED)
async def create_intent(req: IntentRequest, svc: PaymentHandoff = Depends(handoff)) -> PaymentIntent:
    try:
        return await svc.create_intent(req.order_id, req.tip)
    except PaymentRejected as exc:
        raise HTTPException(status.HTTP_409_CONFLICT, str(exc)) from exc


@router.post("/webhook", status_code=status.HTTP_204_NO_CONTENT)
async def provider_webhook(
    request: Request,
    svc: PaymentHandoff = Depends(handoff),
    tillpoint_signature: str = Header(alias="Tillpoint-Signature"),
) -> None:
    try:
        await svc.handle_webhook(await request.body(), tillpoint_signature)
    except PaymentRejected as exc:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, str(exc)) from exc
