"""Consumes filed claims, withdrawals and fraud alerts on one consumer group.

Fraud alerts and withdrawals only update local state; a filed claim is assessed
when it arrives, unless a withdrawal for it was already seen. Offsets are
committed after the outcome is published (at-least-once; consumers downstream
dedupe on eventId)."""
from __future__ import annotations

from pathlib import Path

import structlog
import yaml
from confluent_kafka import Consumer, KafkaError
from confluent_kafka.schema_registry import SchemaRegistryClient
from confluent_kafka.schema_registry.avro import AvroDeserializer
from confluent_kafka.serialization import MessageField, SerializationContext

from .models import FiledClaim, Outcome
from .producer import DecisionPublisher
from .referral import ReferralPublisher
from .rules import assess
from .settings import KafkaSettings

log = structlog.get_logger()

FILED_EVENT_TYPES = ("MotorClaimFiled", "FloodClaimFiled", "FireClaimFiled")


# @gloss implements .AssessmentPipeline conf=0.9
class AssessmentConsumer:
    def __init__(self, kafka: KafkaSettings, consumer_config: Path, decisions: DecisionPublisher,
                 referrals: ReferralPublisher) -> None:
        conf = yaml.safe_load(consumer_config.read_text())["consumer"]
        self._consumer = Consumer({"bootstrap.servers": kafka.bootstrap, **conf})
        self._deserializer = AvroDeserializer(SchemaRegistryClient({"url": kafka.schema_registry}))
        self._kafka = kafka
        self._decisions = decisions
        self._referrals = referrals
        self._fraud_holds: set[str] = set()
        self._withdrawn: set[str] = set()

    def run(self) -> None:
        self._consumer.subscribe([self._kafka.filed_topic, self._kafka.lifecycle_topic, self._kafka.fraud_topic])
        try:
            while True:
                msg = self._consumer.poll(1.0)
                if msg is None:
                    continue
                if msg.error():
                    if msg.error().code() != KafkaError._PARTITION_EOF:
                        log.error("consume_failed", error=str(msg.error()))
                    continue
                self.handle(msg.topic(), self._record_name(msg), self._value(msg))
                self._consumer.commit(message=msg, asynchronous=False)
        finally:
            self._consumer.close()

    # @gloss subscribes claim:MotorClaimFiled, claim:FloodClaimFiled, claim:FireClaimFiled, claim:ClaimWithdrawn, claim:FraudSuspected conf=0.9
    def handle(self, topic: str, record_name: str, value: dict) -> None:
        claim_id = value["claim"]["claimId"]
        if topic == self._kafka.fraud_topic:
            self._fraud_holds.add(claim_id)
        elif topic == self._kafka.lifecycle_topic and record_name == "ClaimWithdrawn":
            self._withdrawn.add(claim_id)
        elif topic == self._kafka.filed_topic and record_name in FILED_EVENT_TYPES:
            if claim_id in self._withdrawn:
                log.info("skip_withdrawn", claim_id=claim_id)
                return
            assessment = assess(FiledClaim.from_event(record_name, value), fraud_hold=claim_id in self._fraud_holds)
            log.info("assessed", claim_id=claim_id, outcome=assessment.outcome, reason=assessment.reason)
            if assessment.outcome is Outcome.REFERRED:
                self._referrals.refer(assessment)
            else:
                self._decisions.publish(assessment)

    def _value(self, msg) -> dict:
        return self._deserializer(msg.value(), SerializationContext(msg.topic(), MessageField.VALUE))

    @staticmethod
    def _record_name(msg) -> str:
        headers = dict(msg.headers() or [])
        return headers.get("record-name", b"").decode() or "unknown"
