# @gloss verifies .ReminderPlanner scope=file conf=0.85
from datetime import UTC, datetime, timedelta
from uuid import uuid4

from reminders.models import AppointmentEvent, Channel
from reminders.scheduler import cancel_reminders, plan_reminders

NOW = datetime(2026, 3, 2, 9, 0, tzinfo=UTC)


def booked(starts_in: timedelta) -> AppointmentEvent:
    return AppointmentEvent("appointment.booked", uuid4(), uuid4(), NOW + starts_in)


def test_booking_a_week_ahead_plans_lead_reminder_and_nudge():
    plan = plan_reminders(booked(timedelta(days=7)), sms_consent=True, now=NOW)
    assert [r.template for r in plan] == ["reminder_lead", "reminder_same_day"]
    assert all(r.channel is Channel.SMS for r in plan)


# @gloss verifies .LeadTimeSetting conf=0.55
def test_booking_tomorrow_skips_the_lead_reminder():
    plan = plan_reminders(booked(timedelta(hours=20)), sms_consent=False, now=NOW)
    assert [r.template for r in plan] == ["reminder_same_day"]
    assert plan[0].channel is Channel.EMAIL


def test_cancellation_withdraws_reminders():
    event = AppointmentEvent("appointment.cancelled", uuid4(), uuid4(), NOW)
    assert cancel_reminders(event)
    assert plan_reminders(event, sms_consent=True, now=NOW) == []
