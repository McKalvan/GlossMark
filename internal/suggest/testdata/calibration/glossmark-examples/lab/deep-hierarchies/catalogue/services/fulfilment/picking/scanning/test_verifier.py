from .verifier import verify_scan


# @gloss verifies spk:ScanVerifier
def test_wrong_part_is_rejected():
    result = verify_scan("RD-R8150", "4550170000001", {"4550170000001": "RD-R7150"})
    assert not result.ok
    assert "bin expects RD-R8150" in result.reason
