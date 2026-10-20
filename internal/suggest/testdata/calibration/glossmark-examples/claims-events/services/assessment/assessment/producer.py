"""Publishes assessment outcomes: approve and deny decisions, and the payout an approval authorizes."""
from __future__ import annotations

import time
import uuid
from datetime import date, timedelta
from pathlib import Path

from confluent_kafka import Producer
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroSerializer
from confluent_kafka.serialization import MessageField, SerializationContext

from .models import Assessment, Outcome
from .settings import KafkaSettings


def _ref(a: Assessment) -> dict:
    c = a.claim
    return {
        "claimId": c.claim_id,
        "policyNumber": c.policy_number,
        "policyholderId": c.policyholder_id,
        "lineOfBusiness": c.line_of_business,
    }


class DecisionPublisher:
    def __init__(self, kafka: KafkaSettings, schema_dir: Path) -> None:
        registry = SchemaRegistryClient({"url": kafka.schema_registry})
        conf = {"auto.register.schemas": False, "subject.name.strategy": _topic_record_name}
        self._serializers = {
            name: AvroSerializer(registry, (schema_dir / f"{name}.avsc").read_text(), conf=conf)
            for name in ("ClaimApproved", "ClaimDenied", "PayoutAuthorized")
        }
        self._producer = Producer({
            "bootstrap.servers": kafka.bootstrap,
            "enable.idempotence": True,
            "acks": "all",
        })
        self._kafka = kafka

    # @gloss publishes claim:ClaimApproved, claim:ClaimDenied, claim:PayoutAuthorized conf=0.9
    # @gloss produces claim:Payout conf=0.85
    def publish(self, assessment: Assessment) -> None:
        if assessment.outcome is Outcome.APPROVED:
            self._send(self._kafka.decisions_topic, "ClaimApproved", assessment, {
                "settledAmount": {"amountMinor": assessment.settled_amount.amount_minor,
                                  "currency": assessment.settled_amount.currency},
                "autoApproved": assessment.decided_by == "rules-engine",
            })
            self._send(self._kafka.payouts_topic, "PayoutAuthorized", assessment, {
                "payoutId": str(uuid.uuid4()),
                "amount": {"amountMinor": assessment.settled_amount.amount_minor,
                           "currency": assessment.settled_amount.currency},
                "payeeIbanLast4": assessment.claim.details.get("payeeIbanLast4", "0000"),
                "expectedValueDate": (date.today() + timedelta(days=2)).toordinal() - date(1970, 1, 1).toordinal(),
            }, decision_fields=False)
        elif assessment.outcome is Outcome.DENIED:
            self._send(self._kafka.decisions_topic, "ClaimDenied", assessment, {
                "denialCode": assessment.reason,
                "explanation": f"Denied by rule {assessment.reason}",
            })
        self._producer.flush(10)

    def _send(self, topic: str, record: str, a: Assessment, fields: dict, *, decision_fields: bool = True) -> None:
        value = {"eventId": str(uuid.uuid4()), "occurredAt": int(time.time() * 1000), "claim": _ref(a), **fields}
        if decision_fields:
            value |= {"assessmentId": a.assessment_id, "decidedBy": a.decided_by}
        payload = self._serializers[record](value, SerializationContext(topic, MessageField.VALUE))
        self._producer.produce(topic, key=a.claim.claim_id, value=payload)


def _topic_record_name(ctx, record_name: str) -> str:
    return f"{ctx.topic}-{record_name}"
