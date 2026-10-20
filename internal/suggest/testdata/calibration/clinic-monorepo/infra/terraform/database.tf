# @gloss provisions .BookingDatabase
resource "aws_db_instance" "booking" {
  engine = "postgres"
}

resource "aws_db_subnet_group" "booking" {
  name = "booking"
}

resource "aws_iam_role" "reminders" {
  name = "reminders"
}

variable "region" {
  default = "eu-west-1"
}
