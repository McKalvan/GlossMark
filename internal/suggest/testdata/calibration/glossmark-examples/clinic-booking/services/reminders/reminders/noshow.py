"""The no-show rate per clinic, the number operations watches every Monday."""
from dataclasses import dataclass


@dataclass(frozen=True)
class NoShowReport:
    attended: int
    no_shows: int
    late_cancellations: int

    @property
    def rate(self) -> float:
        """No-shows plus late cancellations over every appointment that should have happened."""
        total = self.attended + self.no_shows + self.late_cancellations
        return 0.0 if total == 0 else (self.no_shows + self.late_cancellations) / total


# @gloss implements .NoShowReport conf=0.45
def compute_noshow_report(outcomes: list[tuple[str, bool]]) -> NoShowReport:
    attended = sum(1 for status, _ in outcomes if status == "attended")
    no_shows = sum(1 for status, _ in outcomes if status == "no_show")
    late = sum(1 for status, late in outcomes if status == "cancelled" and late)
    return NoShowReport(attended, no_shows, late)
