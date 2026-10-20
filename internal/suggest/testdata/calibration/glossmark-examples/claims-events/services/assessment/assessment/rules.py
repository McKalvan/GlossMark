"""The assessment rules. Pure functions: no Kafka, no clock, so they are tested directly."""
from __future__ import annotations

from .models import Assessment, FiledClaim, Money, Outcome
from .settings import AUTO_APPROVE_LIMIT_EUR, REQUIRED_EVIDENCE

EXCLUDED_FLOOD_SOURCES = {"STORM_SURGE"}


# @gloss implements .AssessmentRules conf=0.9
# @gloss produces claim:Assessment conf=0.85
def assess(claim: FiledClaim, *, fraud_hold: bool) -> Assessment:
    """Decide one filed claim. Denials first, then holds, then the auto-approval limit."""
    if claim.event_type == "FloodClaimFiled" and claim.details.get("floodSource") in EXCLUDED_FLOOD_SOURCES:
        return Assessment(claim, Outcome.DENIED, "EXCLUDED_PERIL")
    if fraud_hold:
        return Assessment(claim, Outcome.REFERRED, "FRAUD_HOLD")
    if len(claim.evidence_ids) < REQUIRED_EVIDENCE[claim.line_of_business]:
        return Assessment(claim, Outcome.REFERRED, "MISSING_EVIDENCE")
    if claim.details.get("thirdPartyInvolved"):
        return Assessment(claim, Outcome.REFERRED, "THIRD_PARTY")
    if claim.estimated_loss.currency != "EUR" or claim.estimated_loss.major > AUTO_APPROVE_LIMIT_EUR:
        return Assessment(claim, Outcome.REFERRED, "ABOVE_AUTO_APPROVE_LIMIT")
    return Assessment(claim, Outcome.APPROVED, "AUTO_APPROVED", settled_amount=_less_excess(claim))


def _less_excess(claim: FiledClaim) -> Money:
    excess_minor = 25_000 if claim.line_of_business == "HOME" else 15_000
    return Money(max(claim.estimated_loss.amount_minor - excess_minor, 0), claim.estimated_loss.currency)
