# @gloss provisions .BookingCluster conf=0.9
module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.24"

  cluster_name    = "clinic-booking-${var.environment}"
  cluster_version = "1.31"
  vpc_id          = var.vpc_id
  subnet_ids      = var.private_subnet_ids

  eks_managed_node_groups = {
    general = {
      instance_types = ["m7g.large"]
      min_size       = 2
      max_size       = 6
      desired_size   = 3
    }
  }
}

resource "aws_iam_role" "reminders" {
  name = "clinic-booking-reminders"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Federated = module.eks.oidc_provider_arn }
      Action    = "sts:AssumeRoleWithWebIdentity"
    }]
  })
}
