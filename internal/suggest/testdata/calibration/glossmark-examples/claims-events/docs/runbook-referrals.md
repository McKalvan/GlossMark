<!-- @gloss documents claim:ClaimReferred scope=file conf=0.8 -->
# Runbook: referral backlog

Adjusters work the queue `property-and-motor` fed by `claims.referrals.v1`.

## The backlog is growing

1. Check the referral reasons in the last hour. A jump in `ABOVE_AUTO_APPROVE_LIMIT`
   after a deploy usually means the auto-approval limit changed.
2. A jump in `FRAUD_HOLD` follows a change to the fraud referral threshold. Compare the
   deployed fraud-screening version with the last reviewed threshold.
3. `MISSING_EVIDENCE` rises when the upload service is degraded; claims re-enter
   assessment when evidence arrives.

## A referred claim must be decided by hand

Adjusters decide in the claims workbench, which publishes `ClaimApproved` or
`ClaimDenied` with `decidedBy` set to the adjuster's id.
