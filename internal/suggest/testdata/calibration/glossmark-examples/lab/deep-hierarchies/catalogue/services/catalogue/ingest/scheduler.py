"""Runs every supplier importer on its own schedule."""
from __future__ import annotations

import datetime as dt
from dataclasses import dataclass
from typing import Callable, Iterable


@dataclass(frozen=True)
class ImportJob:
    supplier: str
    every: dt.timedelta
    run: Callable[[], int]


# @gloss implements spk:IngestScheduler
def due_jobs(jobs: Iterable[ImportJob], last_run: dict[str, dt.datetime], now: dt.datetime) -> list[ImportJob]:
    return [job for job in jobs if now - last_run.get(job.supplier, dt.datetime.min) >= job.every]
