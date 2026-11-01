"""Settings read once at start-up. Business limits live here, not in the environment,
so a change to one is a reviewed code change."""

# Claims at or below this estimated loss, with no fraud hold and complete evidence,
# are approved without an adjuster.
AUTO_APPROVE_LIMIT_EUR = 5_000

# Evidence each line of business needs before the rules may decide.
REQUIRED_EVIDENCE = {
    "MOTOR": 1,
    "HOME": 2,
}
