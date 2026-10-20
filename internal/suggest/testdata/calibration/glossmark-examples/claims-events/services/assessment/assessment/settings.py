"""Settings read once at start-up. Business limits live here, not in the environment,
so a change to one is a reviewed code change."""
import os
from dataclasses import dataclass

# Claims at or below this estimated loss, with no fraud hold and complete evidence,
# are approved without an adjuster.
# @gloss implements .AutoApprovalRule conf=0.9
AUTO_APPROVE_LIMIT_EUR = 5_000

# Evidence each line of business needs before the rules may decide.
REQUIRED_EVIDENCE = {
    "MOTOR": 1,
    "HOME": 2,
}

ADJUSTER_QUEUE = "property-and-motor"


@dataclass(frozen=True)
class KafkaSettings:
    bootstrap: str
    schema_registry: str
    filed_topic: str = "claims.filed.v1"
    lifecycle_topic: str = "claims.lifecycle.v1"
    fraud_topic: str = "fraud.alerts.v1"
    decisions_topic: str = "claims.decisions.v1"
    referrals_topic: str = "claims.referrals.v1"
    payouts_topic: str = "payouts.v1"


def kafka_from_env() -> KafkaSettings:
    return KafkaSettings(
        bootstrap=os.environ["KAFKA_BOOTSTRAP"],
        schema_registry=os.environ["SCHEMA_REGISTRY_URL"],
    )
