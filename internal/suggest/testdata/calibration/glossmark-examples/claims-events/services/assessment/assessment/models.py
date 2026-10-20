"""The assessment record: what the rules concluded about one claim."""
from __future__ import annotations

import enum
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone


class Outcome(enum.StrEnum):
    APPROVED = "APPROVED"
    DENIED = "DENIED"
    REFERRED = "REFERRED"


@dataclass(frozen=True)
class Money:
    amount_minor: int
    currency: str

    @property
    def major(self) -> float:
        return self.amount_minor / 100


# @gloss represents claim:Claim conf=0.85
@dataclass(frozen=True)
class FiledClaim:
    claim_id: str
    policy_number: str
    policyholder_id: str
    line_of_business: str
    event_type: str
    estimated_loss: Money
    evidence_ids: tuple[str, ...]
    details: dict

    @classmethod
    def from_event(cls, event_type: str, value: dict) -> "FiledClaim":
        ref = value["claim"]
        loss = value["estimatedLoss"]
        details = {k: v for k, v in value.items() if k not in {"eventId", "occurredAt", "claim", "estimatedLoss", "evidenceIds"}}
        return cls(
            claim_id=ref["claimId"],
            policy_number=ref["policyNumber"],
            policyholder_id=ref["policyholderId"],
            line_of_business=ref["lineOfBusiness"],
            event_type=event_type,
            estimated_loss=Money(loss["amountMinor"], loss["currency"]),
            evidence_ids=tuple(value.get("evidenceIds", [])),
            details=details,
        )


# @gloss represents claim:Assessment conf=0.95
@dataclass(frozen=True)
class Assessment:
    claim: FiledClaim
    outcome: Outcome
    reason: str
    settled_amount: Money | None = None
    assessment_id: str = field(default_factory=lambda: str(uuid.uuid4()))
    decided_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    decided_by: str = "rules-engine"
