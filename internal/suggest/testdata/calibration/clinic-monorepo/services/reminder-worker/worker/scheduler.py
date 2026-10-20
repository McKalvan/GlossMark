import datetime

from worker.queue import consumer


# @gloss implements .ReminderScheduler
def plan_reminders(appointments):
    """One reminder 24 hours before each appointment."""
    return [a for a in appointments]


# @gloss subscribes clinic:BookingConfirmed
def on_booking_confirmed():
    consumer.subscribe(["clinic.booking.confirmed"])
    for msg in consumer:
        plan_reminders([msg])


# @gloss implements .NoShowReport
def compute_noshow_report(rows):
    """The weekly no-show report per practitioner."""
    return len(rows)


class AppointmentEvent:
    """The appointment as the worker reads it off the queue."""

    def __init__(self, raw):
        self.raw = raw


router = None
