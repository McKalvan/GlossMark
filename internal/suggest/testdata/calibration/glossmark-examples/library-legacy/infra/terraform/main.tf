# Cloud move, 2024 (CIRC-1980): the two on-prem boxes (circ-app01, circ-batch01)
# and the Postgres server became ECS Fargate services and RDS in the city's AWS account.
terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
  backend "s3" {
    bucket         = "riverbend-city-tfstate"
    key            = "library/circ/terraform.tfstate"
    region         = "us-east-2"
    dynamodb_table = "riverbend-city-tflock"
    encrypt        = true
  }
}

provider "aws" {
  region = var.region
  default_tags {
    tags = {
      department = "library"
      system     = "circ"
      managed_by = "terraform"
    }
  }
}

data "aws_vpc" "city" {
  tags = { Name = "city-shared" }
}

data "aws_subnets" "private" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.city.id]
  }
  tags = { tier = "private" }
}
