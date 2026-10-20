# @gloss provisions circ:LoanStore
resource "aws_dynamodb_table" "loans" {
  name = "loans"
  tags = { "gloss:concept" = "circ:LoanStore" }   # optional runtime mirror
}
