from reminders.noshow import compute_noshow_report


# @gloss verifies clinic:NoShowRate conf=0.55
def test_late_cancellations_count_as_no_shows():
    report = compute_noshow_report([("attended", False)] * 8 + [("no_show", False), ("cancelled", True), ("cancelled", False)])
    assert report.no_shows == 1
    assert report.late_cancellations == 1
    assert round(report.rate, 2) == 0.2


def test_empty_clinic_has_zero_rate():
    assert compute_noshow_report([]).rate == 0.0
