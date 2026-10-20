# @gloss provisions circ:LoanStore
resource "aws_dynamodb_table" "loans" {
  name = "loans"
  tags = { "gloss:concept" = "circ:LoanStore" } # optional runtime mirror, not a marker
}

# @gloss provisions .ReturnQueue
resource "aws_sqs_queue" "returns" { name = "return-queue" }
