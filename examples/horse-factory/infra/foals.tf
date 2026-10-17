# @gloss provisions impl:FoalRegistry
resource "aws_dynamodb_table" "foals" {
  name = "foals"
  tags = { "gloss:concept" = "impl:FoalRegistry" }   # optional runtime mirror
}
