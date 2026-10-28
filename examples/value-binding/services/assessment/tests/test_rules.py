# @gloss verifies .AssessmentRules scope=file
from assessment.rules import FiledClaim, Outcome, assess
from assessment.settings import AUTO_APPROVE_LIMIT_EUR


def motor_claim(amount_eur: float, **kw) -> FiledClaim:
    return FiledClaim("MOTOR", amount_eur, evidence_ids=("photo-1",), **kw)


def test_small_motor_claim_is_auto_approved():
    assert assess(motor_claim(1_200), fraud_hold=False).outcome is Outcome.APPROVED


# @gloss verifies claim:AutoApprovalLimit
def test_claim_at_the_limit_is_approved_and_one_cent_above_is_referred():
    assert assess(motor_claim(AUTO_APPROVE_LIMIT_EUR), fraud_hold=False).outcome is Outcome.APPROVED
    above = assess(motor_claim(AUTO_APPROVE_LIMIT_EUR + 0.01), fraud_hold=False)
    assert (above.outcome, above.reason) == (Outcome.REFERRED, "ABOVE_AUTO_APPROVE_LIMIT")


def test_fraud_hold_refers_even_a_small_claim():
    a = assess(motor_claim(200), fraud_hold=True)
    assert (a.outcome, a.reason) == (Outcome.REFERRED, "FRAUD_HOLD")
