import pytest

from assessment.models import FiledClaim, Money


@pytest.fixture
def motor_claim():
    def make(amount_eur: float = 1_200, evidence: int = 1, third_party: bool = False) -> FiledClaim:
        return FiledClaim(
            claim_id="c-motor-1",
            policy_number="MTR-100234",
            policyholder_id="h-1",
            line_of_business="MOTOR",
            event_type="MotorClaimFiled",
            estimated_loss=Money(int(amount_eur * 100), "EUR"),
            evidence_ids=tuple(f"ev-{i}" for i in range(evidence)),
            details={"thirdPartyInvolved": third_party, "incidentKind": "GLASS"},
        )
    return make


@pytest.fixture
def flood_claim():
    def make(source: str = "RIVER", amount_eur: float = 3_000) -> FiledClaim:
        return FiledClaim(
            claim_id="c-flood-1",
            policy_number="HOM-55120",
            policyholder_id="h-2",
            line_of_business="HOME",
            event_type="FloodClaimFiled",
            estimated_loss=Money(int(amount_eur * 100), "EUR"),
            evidence_ids=("photo-1", "photo-2"),
            details={"floodSource": source, "inFloodZone": True},
        )
    return make
