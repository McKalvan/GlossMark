"""Persistence model for expense claims."""
from __future__ import annotations

import enum
from dataclasses import dataclass, field
from datetime import datetime, timezone
from decimal import Decimal
from uuid import UUID, uuid4


class ClaimStatus(str, enum.Enum):
    SUBMITTED = "submitted"
    APPROVED = "approved"
    REJECTED = "rejected"


# @gloss represents exp:ExpenseClaim
@dataclass
class Claim:
    claimant_id: str
    amount: Decimal
    currency: str
    description: str
    id: UUID = field(default_factory=uuid4)
    status: ClaimStatus = ClaimStatus.SUBMITTED
    decided_by: str | None = None
    submitted_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))

    def to_json(self) -> dict:
        return {
            "id": str(self.id),
            "claimant_id": self.claimant_id,
            "amount": str(self.amount),
            "currency": self.currency,
            "description": self.description,
            "status": self.status.value,
            "decided_by": self.decided_by,
            "submitted_at": self.submitted_at.isoformat(),
        }
