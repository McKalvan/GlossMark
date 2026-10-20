<!-- @gloss documents circ:NightlyBatch scope=file conf=0.85 -->
# Runbook: nightly batch

The nightly batch runs at 01:30 Central as the `circ-nightly` ECS scheduled task (until March 2024 it was cron on circ-batch01; the old crontab is in `batch/crontab`).

## What it does, in order

1. `assess_fines.rb`: accrues overdue fines for items still out (`sp_assess_fines`).
2. `overdue_notices.pl`: emails or prints notices at 7, 14 and 28 days; marks items lost at 60 days and bills them.
3. `expire_holds.sh`: expires trapped holds past their pickup date and re-traps the copy (`sp_expire_holds`).

Fines run before notices so a notice shows the balance as of tonight.

## When it fails

- **Fines failed, notices did not run.** The script stops at the first error (`set -eu`). Rerun the whole batch for the same date: `run_nightly.sh` is safe to repeat because `sp_assess_fines` upserts one overdue fine per loan.
- **Notices sent twice.** Only possible if the batch is run for the same date after the 7/14/28-day stage moved on. Check `/var/log/circ/nightly-<date>.log`.
- **Printed notices missing.** The Main branch mail run picks up `/var/spool/circ/notices/branch-1` at 09:00.

Rerun for a given date:

```sh
aws ecs run-task --cluster circ-prod --task-definition circ-web \
  --overrides '{"containerOverrides":[{"name":"circ-web","command":["/opt/circ/batch/bin/run_nightly.sh"]}]}'
```
