variable "region" {
  type    = string
  default = "eu-west-2"
}

variable "environment" {
  type    = string
  default = "prod"
}

variable "vpc_id" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "db_instance_class" {
  type    = string
  default = "db.r6g.large"
}
