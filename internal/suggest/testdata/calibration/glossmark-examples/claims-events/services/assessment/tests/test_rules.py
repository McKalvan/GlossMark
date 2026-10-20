# @gloss verifies .AssessmentRules scope=file conf=0.9
from assessment.models import Outcome
from assessment.rules import assess
from assessment.settings import AUTO_APPROVE_LIMIT_EUR


def test_small_motor_claim_is_auto_approved_less_excess(motor_claim):
    a = assess(motor_claim(amount_eur=1_200), fraud_hold=False)
    assert a.outcome is Outcome.APPROVED
    assert a.settled_amount.amount_minor == 120_000 - 15_000


# @gloss verifies claim:AutoApprovalLimit conf=0.9
def test_claim_at_the_limit_is_approved_and_one_cent_above_is_referred(motor_claim):
    assert assess(motor_claim(amount_eur=AUTO_APPROVE_LIMIT_EUR), fraud_hold=False).outcome is Outcome.APPROVED
    above = assess(motor_claim(amount_eur=AUTO_APPROVE_LIMIT_EUR + 0.01), fraud_hold=False)
    assert (above.outcome, above.reason) == (Outcome.REFERRED, "ABOVE_AUTO_APPROVE_LIMIT")


def test_fraud_hold_refers_even_a_small_claim(motor_claim):
    a = assess(motor_claim(amount_eur=200), fraud_hold=True)
    assert (a.outcome, a.reason) == (Outcome.REFERRED, "FRAUD_HOLD")


def test_storm_surge_flood_is_an_excluded_peril(flood_claim):
    a = assess(flood_claim(source="STORM_SURGE"), fraud_hold=False)
    assert (a.outcome, a.reason) == (Outcome.DENIED, "EXCLUDED_PERIL")


def test_third_party_motor_claims_go_to_an_adjuster(motor_claim):
    assert assess(motor_claim(third_party=True), fraud_hold=False).reason == "THIRD_PARTY"


def test_home_claim_needs_two_pieces_of_evidence(flood_claim):
    assert assess(flood_claim(), fraud_hold=False).outcome is Outcome.APPROVED
