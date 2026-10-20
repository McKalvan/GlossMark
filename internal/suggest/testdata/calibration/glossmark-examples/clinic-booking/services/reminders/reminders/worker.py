"""Long-running worker: consume booking events, send what is due."""
import json
import logging
import time
from datetime import UTC, datetime
from uuid import UUID

import boto3

from .channels import sender_for
from .models import AppointmentEvent
from .repository import ReminderRepository
from .scheduler import cancel_reminders, plan_reminders
from .settings import Settings
from .templates import render

log = logging.getLogger(__name__)


def parse_event(body: str, kind: str) -> AppointmentEvent:
    data = json.loads(body)
    return AppointmentEvent(
        kind=kind,
        appointment_id=UUID(data["id"]),
        patient_id=UUID(data["patientId"]),
        starts_at=datetime.fromisoformat(data["startsAt"]),
    )


# @gloss implements .ReminderDispatch conf=0.45
class ReminderWorker:
    def __init__(self, settings: Settings, repo: ReminderRepository):
        self._settings = settings
        self._repo = repo
        self._sqs = boto3.client("sqs")

    def consume_once(self) -> int:
        resp = self._sqs.receive_message(
            QueueUrl=self._settings.queue_url, MaxNumberOfMessages=10, WaitTimeSeconds=10,
            MessageAttributeNames=["kind"],
        )
        handled = 0
        for msg in resp.get("Messages", []):
            event = parse_event(msg["Body"], msg["MessageAttributes"]["kind"]["StringValue"])
            if cancel_reminders(event):
                self._repo.withdraw(event.appointment_id)
            else:
                consent = self._repo.sms_consent(event.patient_id)
                self._repo.save(plan_reminders(event, consent, datetime.now(UTC)))
            self._sqs.delete_message(QueueUrl=self._settings.queue_url, ReceiptHandle=msg["ReceiptHandle"])
            handled += 1
        return handled

    def send_due(self) -> int:
        sent = 0
        now = datetime.now(UTC)
        for reminder in self._repo.due(now, self._settings.batch_size):
            body = render(reminder.template, starts_at=reminder.send_at)
            if not self._settings.dry_run:
                sender = sender_for(reminder.channel, self._settings.sms_sender_id, self._settings.email_from)
                sender.send(str(reminder.patient_id), body)
            self._repo.mark_sent(reminder, now)
            sent += 1
        return sent

    def run_forever(self) -> None:
        while True:
            handled = self.consume_once()
            sent = self.send_due()
            log.info("cycle", extra={"events": handled, "sent": sent})
            if not handled and not sent:
                time.sleep(5)
