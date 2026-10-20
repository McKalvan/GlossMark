import hashlib
import hmac
from decimal import Decimal

from ordering.services.payment_handoff import split_evenly, verify_signature


# @gloss verifies .PaymentHandoff conf=0.9
def test_webhook_signature_round_trip():
    body = b'{"type":"intent.captured"}'
    sig = hmac.new(b"whsec", body, hashlib.sha256).hexdigest()
    assert verify_signature(body, sig, "whsec")
    assert not verify_signature(body, sig, "other")


def test_split_evenly_puts_pennies_on_the_first_share():
    assert split_evenly(Decimal("100.00"), 3) == [Decimal("33.34"), Decimal("33.33"), Decimal("33.33")]
