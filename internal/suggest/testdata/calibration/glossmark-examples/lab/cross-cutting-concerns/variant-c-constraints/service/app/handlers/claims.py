"""Expense claim endpoints: submit, read, approve, reject."""
from __future__ import annotations

from decimal import Decimal, InvalidOperation
from uuid import UUID

from flask import Blueprint, abort, g, request

from app import store
from app.models import Claim, ClaimStatus
from app.platform.auth import requires_role
from app.platform.ratelimit import rate_limit

bp = Blueprint("claims", __name__)


# @gloss implements .SubmitClaimHandler
@bp.post("")
@rate_limit("claims-write")
def submit_claim():
    body = request.get_json(force=True)
    try:
        amount = Decimal(str(body["amount"]))
    except (KeyError, InvalidOperation):
        abort(400, "amount is required")
    if amount <= 0:
        abort(422, "amount must be positive")
    claim = Claim(
        claimant_id=g.principal.subject,
        amount=amount,
        currency=body.get("currency", "EUR"),
        description=body.get("description", ""),
    )
    return store.save(claim).to_json(), 201


# @gloss implements .ClaimQueries
@bp.get("")
def list_my_claims():
    return {"claims": [c.to_json() for c in store.for_claimant(g.principal.subject)]}


# @gloss implements .ClaimQueries
@bp.get("/<uuid:claim_id>")
def get_claim(claim_id: UUID):
    claim = store.get(claim_id) or abort(404)
    if claim.claimant_id != g.principal.subject and "approver" not in g.principal.roles:
        abort(404)
    return claim.to_json()


# @gloss implements .ClaimDecisionHandler
@bp.post("/<uuid:claim_id>/approve")
@requires_role("approver")
@rate_limit("claims-write")
def approve_claim(claim_id: UUID):
    return _decide(claim_id, ClaimStatus.APPROVED)


# @gloss implements .ClaimDecisionHandler
@bp.post("/<uuid:claim_id>/reject")
@requires_role("approver")
@rate_limit("claims-write")
def reject_claim(claim_id: UUID):
    return _decide(claim_id, ClaimStatus.REJECTED)


def _decide(claim_id: UUID, status: ClaimStatus):
    claim = store.get(claim_id) or abort(404)
    if claim.status is not ClaimStatus.SUBMITTED:
        abort(409, "claim already decided")
    if claim.claimant_id == g.principal.subject:
        abort(403, "approvers may not decide their own claims")
    claim.status = status
    claim.decided_by = g.principal.subject
    return store.save(claim).to_json()
