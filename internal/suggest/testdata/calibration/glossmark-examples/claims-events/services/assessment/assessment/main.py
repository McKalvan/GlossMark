"""Entry point: wire settings, publishers and the consumer, expose metrics, run."""
import os
from pathlib import Path

import structlog
from prometheus_client import start_http_server

from .consumer import AssessmentConsumer
from .producer import DecisionPublisher
from .referral import ReferralPublisher
from .settings import kafka_from_env


def run() -> None:
    structlog.configure(processors=[structlog.processors.JSONRenderer()])
    kafka = kafka_from_env()
    schemas = Path(os.environ.get("SCHEMA_DIR", "/schemas"))
    start_http_server(9102)
    AssessmentConsumer(
        kafka,
        Path(os.environ.get("CONSUMER_CONFIG", "config/consumer-group.yaml")),
        DecisionPublisher(kafka, schemas),
        ReferralPublisher(kafka, schemas),
    ).run()


if __name__ == "__main__":
    run()
