<!-- @gloss documents lark:ReminderQueue scope=file conf=0.8 -->
# Runbook: reminders not arriving

1. Check the dead-letter queue `clinic-booking-reminder-events-dlq.fifo`. Messages there failed five times.
2. Check `outbox` for rows with `sent_at IS NULL` older than five minutes; the relay in the booking API is stuck if there are many.
3. Check the worker logs for `cycle` lines. `sent=0` for an hour during clinic hours means the SNS sender is failing.
4. Set `REMINDERS_DRY_RUN=true` in the reminders ConfigMap to stop sending while you investigate.
