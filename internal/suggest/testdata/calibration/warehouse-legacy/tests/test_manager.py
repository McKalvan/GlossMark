from core.Manager import Manager
from helpers import process_data


def make_rows():
    return [("a", 1, 2)]


# @gloss verifies .ReplenishmentRun
def test_doIt():
    Manager().doIt("A")


# @gloss verifies wh:CycleCount
def test_process_data():
    assert process_data(make_rows())


# @gloss verifies .WaveBatch
def test_run_batch():
    assert Manager().run_batch("18:00")
