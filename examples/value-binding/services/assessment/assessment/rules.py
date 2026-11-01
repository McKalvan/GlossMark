"""The assessment rules. Pure functions: no Kafka, no clock, so they are tested directly."""
from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from .settings import AUTO_APPROVE_LIMIT_EUR, REQUIRED_EVIDENCE


class Outcome(str, Enum):
    APPROVED = "APPROVED"
    DENIED = "DENIED"
    REFERRED = "REFERRED"


@dataclass(frozen=True)
class FiledClaim:
    line_of_business: str
    estimated_loss_eur: float
    evidence_ids: tuple[str, ...] = ()
    third_party_involved: bool = False


# @gloss represents claim:Assessment
@dataclass(frozen=True)
class Assessment:
    claim: FiledClaim
    outcome: Outcome
    reason: str


# @gloss implements .AssessmentRules
# @gloss produces claim:Assessment
def assess(claim: FiledClaim, *, fraud_hold: bool) -> Assessment:
    """Decide one filed claim. Holds first, then evidence, then the auto-approval limit."""
    if fraud_hold:
        return Assessment(claim, Outcome.REFERRED, "FRAUD_HOLD")
    if len(claim.evidence_ids) < REQUIRED_EVIDENCE[claim.line_of_business]:
        return Assessment(claim, Outcome.REFERRED, "MISSING_EVIDENCE")
    if claim.third_party_involved:
        return Assessment(claim, Outcome.REFERRED, "THIRD_PARTY")
    if claim.estimated_loss_eur > AUTO_APPROVE_LIMIT_EUR:
        return Assessment(claim, Outcome.REFERRED, "ABOVE_AUTO_APPROVE_LIMIT")
    return Assessment(claim, Outcome.APPROVED, "AUTO_APPROVED")
