# @gloss verifies .AssessmentPipeline scope=file conf=0.85
"""The consumer's routing, with the Kafka client and publishers replaced by fakes."""
from unittest.mock import MagicMock

import pytest

from assessment import consumer as consumer_module
from assessment.consumer import AssessmentConsumer
from assessment.models import Outcome
from assessment.settings import KafkaSettings

KAFKA = KafkaSettings(bootstrap="unused:9092", schema_registry="http://unused")


@pytest.fixture
def wired(monkeypatch, tmp_path):
    cfg = tmp_path / "consumer-group.yaml"
    cfg.write_text("consumer:\n  group.id: test\n")
    monkeypatch.setattr(consumer_module, "Consumer", MagicMock())
    monkeypatch.setattr(consumer_module, "AvroDeserializer", MagicMock())
    monkeypatch.setattr(consumer_module, "SchemaRegistryClient", MagicMock())
    decisions, referrals = MagicMock(), MagicMock()
    return AssessmentConsumer(KAFKA, cfg, decisions, referrals), decisions, referrals


def _filed(claim_id: str, amount_minor: int = 50_000) -> dict:
    return {
        "claim": {"claimId": claim_id, "policyNumber": "MTR-1", "policyholderId": "h", "lineOfBusiness": "MOTOR"},
        "estimatedLoss": {"amountMinor": amount_minor, "currency": "EUR"},
        "evidenceIds": ["ev-1"],
        "thirdPartyInvolved": False,
    }


def test_filed_claim_is_decided_and_published(wired):
    c, decisions, referrals = wired
    c.handle("claims.filed.v1", "MotorClaimFiled", _filed("c1"))
    assert decisions.publish.call_args.args[0].outcome is Outcome.APPROVED
    referrals.refer.assert_not_called()


def test_fraud_alert_seen_first_turns_approval_into_referral(wired):
    c, decisions, referrals = wired
    c.handle("fraud.alerts.v1", "FraudSuspected", {"claim": {"claimId": "c2"}})
    c.handle("claims.filed.v1", "MotorClaimFiled", _filed("c2"))
    assert referrals.refer.call_args.args[0].reason == "FRAUD_HOLD"
    decisions.publish.assert_not_called()


def test_withdrawn_claim_is_not_assessed(wired):
    c, decisions, referrals = wired
    c.handle("claims.lifecycle.v1", "ClaimWithdrawn", {"claim": {"claimId": "c3"}})
    c.handle("claims.filed.v1", "MotorClaimFiled", _filed("c3"))
    decisions.publish.assert_not_called()
    referrals.refer.assert_not_called()
