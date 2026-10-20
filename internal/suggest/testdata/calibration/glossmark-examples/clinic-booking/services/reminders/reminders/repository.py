"""Postgres access for scheduled reminders and appointment history."""
from collections.abc import Iterator
from datetime import date, datetime
from uuid import UUID

import psycopg

from .models import Channel, ScheduledReminder


class ReminderRepository:
    def __init__(self, conn: psycopg.Connection):
        self._conn = conn

    def sms_consent(self, patient_id: UUID) -> bool:
        row = self._conn.execute("SELECT sms_consent FROM patients WHERE id = %s", (patient_id,)).fetchone()
        return bool(row and row[0])

    def save(self, reminders: list[ScheduledReminder]) -> None:
        with self._conn.cursor() as cur:
            cur.executemany(
                """INSERT INTO reminders (appointment_id, patient_id, channel, send_at, template)
                   VALUES (%s, %s, %s, %s, %s) ON CONFLICT (appointment_id, template) DO NOTHING""",
                [(r.appointment_id, r.patient_id, r.channel.value, r.send_at, r.template) for r in reminders],
            )

    def withdraw(self, appointment_id: UUID) -> None:
        self._conn.execute("DELETE FROM reminders WHERE appointment_id = %s AND sent_at IS NULL", (appointment_id,))

    def due(self, now: datetime, limit: int) -> Iterator[ScheduledReminder]:
        rows = self._conn.execute(
            """SELECT appointment_id, patient_id, channel, send_at, template FROM reminders
               WHERE sent_at IS NULL AND send_at <= %s ORDER BY send_at LIMIT %s FOR UPDATE SKIP LOCKED""",
            (now, limit),
        )
        for a, p, ch, at, t in rows:
            yield ScheduledReminder(a, p, Channel(ch), at, t)

    def mark_sent(self, reminder: ScheduledReminder, sent_at: datetime) -> None:
        self._conn.execute(
            "UPDATE reminders SET sent_at = %s WHERE appointment_id = %s AND template = %s",
            (sent_at, reminder.appointment_id, reminder.template),
        )

    def appointment_outcomes(self, clinic_id: UUID, since: date) -> list[tuple[str, bool]]:
        return self._conn.execute(
            """SELECT a.status, a.late_cancel FROM appointments a JOIN slots s ON s.id = a.slot_id
               WHERE s.clinic_id = %s AND a.starts_at >= %s AND a.starts_at < now()""",
            (clinic_id, since),
        ).fetchall()
