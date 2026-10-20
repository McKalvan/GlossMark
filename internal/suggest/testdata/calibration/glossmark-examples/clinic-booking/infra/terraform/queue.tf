resource "aws_sqs_queue" "reminder_events_dlq" {
  name                      = "clinic-booking-reminder-events-dlq.fifo"
  fifo_queue                = true
  message_retention_seconds = 1209600
}

# @gloss provisions .ReminderQueue conf=0.9
resource "aws_sqs_queue" "reminder_events" {
  name                        = "clinic-booking-reminder-events.fifo"
  fifo_queue                  = true
  content_based_deduplication = false
  visibility_timeout_seconds  = 60
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.reminder_events_dlq.arn
    maxReceiveCount     = 5
  })
}
