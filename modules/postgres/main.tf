
resource "aws_db_instance" "database" {
  identifier           = "${var.project}-postgres-${var.env}"
  engine               = "postgres"
  engine_version       = "14"
  instance_class       = var.instance
  allocated_storage    = var.storage
  username             = var.username
  password             = aws_ssm_parameter.postgres_password.value
  skip_final_snapshot  = true
}
