"""Records the worker reads from the queue and the database."""
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from uuid import UUID


class Channel(StrEnum):
    SMS = "sms"
    EMAIL = "email"


# @gloss consumes clinic:Appointment conf=0.64
@dataclass(frozen=True)
class AppointmentEvent:
    kind: str
    appointment_id: UUID
    patient_id: UUID
    starts_at: datetime


@dataclass(frozen=True)
class ScheduledReminder:
    appointment_id: UUID
    patient_id: UUID
    channel: Channel
    send_at: datetime
    template: str
