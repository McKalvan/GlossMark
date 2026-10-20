"""Turn booking events into scheduled reminders."""
from datetime import datetime, timedelta

from .models import AppointmentEvent, Channel, ScheduledReminder
from .settings import REMINDER_LEAD_HOURS, SAME_DAY_NUDGE_HOURS


# @gloss implements .ReminderPlanner conf=0.45
def plan_reminders(event: AppointmentEvent, sms_consent: bool, now: datetime) -> list[ScheduledReminder]:
    """Reminders for a booked appointment: one at the lead time, one nudge on the day.

    A reminder whose send time has already passed is skipped, so a same-week
    booking only gets the nudge.
    """
    if event.kind != "appointment.booked":
        return []
    channel = Channel.SMS if sms_consent else Channel.EMAIL
    plan = [
        (event.starts_at - timedelta(hours=REMINDER_LEAD_HOURS), "reminder_lead"),
        (event.starts_at - timedelta(hours=SAME_DAY_NUDGE_HOURS), "reminder_same_day"),
    ]
    return [
        ScheduledReminder(event.appointment_id, event.patient_id, channel, send_at, template)
        for send_at, template in plan
        if send_at > now
    ]


def cancel_reminders(event: AppointmentEvent) -> bool:
    """True when the event withdraws reminders already scheduled for the appointment."""
    return event.kind == "appointment.cancelled"
