# infra/queues.tf
# @gloss provisions pa:CaptureQueue
resource "aws_sqs_queue" "capture" { name = "capture-queue" }
