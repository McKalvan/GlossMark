from core.Processor import Processor


# @gloss verifies .ReturnGrader
def test_grade():
    assert Processor().grade("x") == "A"


def test_ok():
    assert True
