from unittest import mock

import helpers


# @gloss verifies wh:CountVariance conf=0.85
def test_calc():
    assert helpers.calc(98, 100) == -0.02
    assert helpers.calc(5, 0) == 5.0


# @gloss verifies wh:CountTolerance conf=0.9
def test_chk():
    assert not helpers.chk(0.02)
    assert helpers.chk(0.021)


# @gloss verifies .CountReconciler conf=0.9
@mock.patch("helpers.save_stuff")
@mock.patch("helpers.get_stuff", return_value=[{"qty": 100}])
def test_process_data(get_stuff, save_stuff):
    rows = [{"loc": "A-01-01", "item": "100200", "qty": "90"}, {"loc": "A-01-02", "item": "100201", "qty": "100"}]
    out = helpers.process_data(rows, None)
    assert [r["loc"] for r in out] == ["A-01-01"]
    assert save_stuff.call_count == 2


def test_it_works():
    assert helpers.fmt_row({"loc": "A", "item": "B", "found": 1, "have": 1, "diff": 0.0})
