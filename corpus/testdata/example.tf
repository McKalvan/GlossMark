# @gloss provisions impl:FoalRegistry
resource "aws_dynamodb_table" "foals" {
  name = "foals"
  tags = { "gloss:concept" = "impl:FoalRegistry" } # optional runtime mirror, not a marker
}

# @gloss provisions .CaptureQueue
resource "aws_sqs_queue" "capture" { name = "capture-queue" }
