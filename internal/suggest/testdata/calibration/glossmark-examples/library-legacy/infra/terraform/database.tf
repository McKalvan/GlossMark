resource "aws_db_subnet_group" "circ" {
  name       = "circ-${var.environment}"
  subnet_ids = data.aws_subnets.private.ids
}

# @gloss provisions .CircDatabase conf=0.9
resource "aws_db_instance" "circ" {
  identifier                  = "circ-${var.environment}"
  engine                      = "postgres"
  engine_version              = "15.6"
  instance_class              = var.db_instance_class
  allocated_storage           = 100
  db_name                     = "circ"
  username                    = "circ_admin"
  manage_master_user_password = true
  db_subnet_group_name        = aws_db_subnet_group.circ.name
  backup_retention_period     = 14
  deletion_protection         = true
  storage_encrypted           = true
  skip_final_snapshot         = false
  final_snapshot_identifier   = "circ-${var.environment}-final"
}
