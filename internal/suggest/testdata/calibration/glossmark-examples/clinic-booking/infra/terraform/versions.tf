terraform {
  required_version = ">= 1.8"
  required_providers {
    aws = { source = "hashicorp/aws", version = "~> 5.70" }
  }
  backend "s3" {
    bucket         = "larkspur-terraform-state"
    key            = "clinic-booking/prod.tfstate"
    region         = "eu-west-2"
    dynamodb_table = "terraform-locks"
  }
}

provider "aws" {
  region = var.region
  default_tags { tags = { system = "clinic-booking", owner = "platform" } }
}
