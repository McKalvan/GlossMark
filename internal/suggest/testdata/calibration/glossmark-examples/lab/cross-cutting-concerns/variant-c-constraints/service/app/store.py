"""In-process claim store; replaced by Postgres in production builds."""
from __future__ import annotations

from uuid import UUID

from app.models import Claim

_claims: dict[UUID, Claim] = {}


def save(claim: Claim) -> Claim:
    _claims[claim.id] = claim
    return claim


def get(claim_id: UUID) -> Claim | None:
    return _claims.get(claim_id)


def for_claimant(claimant_id: str) -> list[Claim]:
    return [c for c in _claims.values() if c.claimant_id == claimant_id]


def all_claims() -> list[Claim]:
    return sorted(_claims.values(), key=lambda c: c.submitted_at)
