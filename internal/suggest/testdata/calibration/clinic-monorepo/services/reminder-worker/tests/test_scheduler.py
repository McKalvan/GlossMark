# @gloss verifies .ReminderScheduler scope=file
import pytest

from worker.scheduler import plan_reminders


@pytest.fixture
def appointment():
    return {"id": "a1"}


def test_plan_reminders_one_per_appointment(appointment):
    assert len(plan_reminders([appointment])) == 1


def test_plan_reminders_empty():
    assert plan_reminders([]) == []


def test_plan_reminders_keeps_order():
    assert plan_reminders([1, 2]) == [1, 2]
