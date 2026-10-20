# Claims event catalog

Every event the claims platform exchanges, who publishes it and who reads it. The
contract is [contracts/asyncapi.yaml](../contracts/asyncapi.yaml); the payloads are the
Avro schemas in [schemas/](../schemas).

<!-- @gloss documents claim:ClaimFiled conf=0.85 -->
## Filed claims

`claims.filed.v1` carries one event per new claim, keyed by claim id. The record type
says which line of business: `MotorClaimFiled`, `FloodClaimFiled` or `FireClaimFiled`.
Published by claim-intake after the claim is stored. Read by assessment and by fraud
screening, each on its own consumer group.

<!-- @gloss documents claim:ClaimWithdrawn conf=0.85 -->
## Withdrawals

`claims.lifecycle.v1` carries `ClaimWithdrawn` when a policyholder withdraws a claim
before a decision. Assessment skips a claim it has seen withdrawn.

<!-- @gloss documents claim:ClaimAssessment conf=0.85 -->
## Decisions and payouts

Assessment publishes `ClaimApproved` or `ClaimDenied` on `claims.decisions.v1`, and a
`PayoutAuthorized` on `payouts.v1` for each approval. Treasury's payment gateway, which
lives outside this repository, publishes `PayoutFailed` on the same topic when a bank
rejects a payment. The payout notifier emails the policyholder about authorized payouts.

<!-- @gloss documents claim:FraudScreening conf=0.85 -->
## Fraud alerts

Fraud screening publishes `FraudSuspected` on `fraud.alerts.v1` for any filed claim that
scores at or above the referral threshold. Assessment holds such a claim and refers it to
an adjuster instead of deciding it.

<!-- @gloss documents claim:ClaimReferred conf=0.85 -->
## Referrals

Claims the rules cannot decide go to adjusters on `claims.referrals.v1` as
`ClaimReferred`.
