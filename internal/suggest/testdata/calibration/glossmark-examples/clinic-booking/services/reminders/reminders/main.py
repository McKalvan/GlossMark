"""Entry points for the worker Deployment and the weekly report CronJob."""
import logging
import sys
from datetime import date, timedelta
from uuid import UUID

import psycopg

from .noshow import compute_noshow_report
from .repository import ReminderRepository
from .settings import Settings
from .worker import ReminderWorker


# @gloss implements .ReminderService conf=0.8
def run_worker() -> None:
    logging.basicConfig(level=logging.INFO)
    settings = Settings()
    with psycopg.connect(settings.database_url, autocommit=True) as conn:
        ReminderWorker(settings, ReminderRepository(conn)).run_forever()


def run_noshow_report() -> None:
    settings = Settings()
    clinic_ids = [UUID(arg) for arg in sys.argv[1:]]
    since = date.today() - timedelta(days=28)
    with psycopg.connect(settings.database_url) as conn:
        repo = ReminderRepository(conn)
        for clinic_id in clinic_ids:
            report = compute_noshow_report(repo.appointment_outcomes(clinic_id, since))
            print(f"{clinic_id}\t{report.rate:.1%}\tattended={report.attended}\tno_show={report.no_shows}\tlate={report.late_cancellations}")
