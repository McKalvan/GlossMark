resource "aws_db_subnet_group" "booking" {
  name       = "clinic-booking-${var.environment}"
  subnet_ids = var.private_subnet_ids
}

resource "aws_kms_key" "patient_data" {
  description         = "Encrypts the booking database (patient PII)"
  enable_key_rotation = true
}

# @gloss provisions .BookingDatabase conf=0.9
resource "aws_db_instance" "booking" {
  identifier                  = "clinic-booking-${var.environment}"
  engine                      = "postgres"
  engine_version              = "16.4"
  instance_class              = var.db_instance_class
  allocated_storage           = 100
  storage_encrypted           = true
  kms_key_id                  = aws_kms_key.patient_data.arn
  db_subnet_group_name        = aws_db_subnet_group.booking.name
  multi_az                    = true
  backup_retention_period     = 35
  deletion_protection         = true
  username                    = "booking_admin"
  manage_master_user_password = true
  performance_insights_enabled = true
}
