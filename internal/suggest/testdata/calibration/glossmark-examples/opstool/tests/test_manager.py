from unittest import mock

from core.Manager import Manager


class Cfg:
    def getint(self, *a, **k):
        return 40

    def get(self, *a, **k):
        return "sup@acme.local"


# @gloss verifies .ReplenishmentRun conf=0.9
@mock.patch("core.Manager.save_stuff")
@mock.patch("core.Manager.get_stuff")
def test_doIt(get_stuff, save_stuff):
    get_stuff.side_effect = [
        [{"loc": "P-01", "item": "100200", "qty": 2, "min_qty": 5, "max_qty": 20}],
        [{"loc": "R-09", "qty": 100}],
    ]
    assert Manager(Cfg()).doIt() == {"moves": 1}
    assert save_stuff.call_args[0][1][:4] == ("R-09", "P-01", "100200", 18)


# @gloss verifies wh:HazmatHandling conf=0.9
def test_ok():
    m = Manager(Cfg())
    assert not m._ok([{"cls": "3"}], [{"cls": "5.1"}])
    assert m._ok([{"cls": "3"}], [{"cls": None}])
