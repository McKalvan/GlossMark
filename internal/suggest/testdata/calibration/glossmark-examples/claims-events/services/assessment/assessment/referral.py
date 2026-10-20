"""Refers a claim the rules cannot decide to the adjuster queue.

Added in September 2026 so adjusters get a feed instead of polling the claims
database. It writes to claims.referrals.v1 with the producer settings the
decision publisher uses."""
from __future__ import annotations

import time
import uuid
from pathlib import Path

from confluent_kafka import Producer
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroSerializer
from confluent_kafka.serialization import MessageField, SerializationContext

from .models import Assessment, Outcome
from .settings import ADJUSTER_QUEUE, KafkaSettings


class ReferralPublisher:
    def __init__(self, kafka: KafkaSettings, schema_dir: Path) -> None:
        registry = SchemaRegistryClient({"url": kafka.schema_registry})
        self._serializer = AvroSerializer(
            registry,
            (schema_dir / "ClaimReferred.avsc").read_text(),
            conf={"auto.register.schemas": False,
                  "subject.name.strategy": lambda ctx, name: f"{ctx.topic}-{name}"},
        )
        self._producer = Producer({"bootstrap.servers": kafka.bootstrap, "enable.idempotence": True, "acks": "all"})
        self._topic = kafka.referrals_topic

    # @gloss publishes claim:ClaimReferred conf=0.9
    def refer(self, assessment: Assessment) -> None:
        if assessment.outcome is not Outcome.REFERRED:
            raise ValueError(f"only referred assessments go to adjusters, got {assessment.outcome}")
        c = assessment.claim
        value = {
            "eventId": str(uuid.uuid4()),
            "occurredAt": int(time.time() * 1000),
            "claim": {"claimId": c.claim_id, "policyNumber": c.policy_number,
                      "policyholderId": c.policyholder_id, "lineOfBusiness": c.line_of_business},
            "assessmentId": assessment.assessment_id,
            "referralReason": assessment.reason,
            "queue": ADJUSTER_QUEUE,
            "decidedBy": assessment.decided_by,
        }
        payload = self._serializer(value, SerializationContext(self._topic, MessageField.VALUE))
        self._producer.produce(self._topic, key=c.claim_id, value=payload)
        self._producer.flush(10)
