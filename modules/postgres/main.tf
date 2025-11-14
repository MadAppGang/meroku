# Map user-friendly versions to Aurora-specific versions
# These are the latest available versions as of Jan 2025
# AWS will auto-update to newer minor versions within the same major version
locals {
  aurora_version_map = {
    "17" = "17.5"  # Latest PostgreSQL 17 with 256 TiB storage support
    "16" = "16.9"  # Latest PostgreSQL 16 with 256 TiB storage support
    "15" = "15.13" # Latest PostgreSQL 15 with 256 TiB storage support
    "14" = "14.18" # Latest PostgreSQL 14
    "13" = "13.18" # Latest PostgreSQL 13 (approaching end of support)
  }

  # Use default values if variables are empty
  db_username = var.username != "" ? var.username : "postgres"
  db_name     = var.db_name != "" ? var.db_name : var.project
}

# Standard RDS Instance (when aurora = false)
resource "aws_db_instance" "database" {
  count          = var.aurora ? 0 : 1
  identifier     = "${var.project}-postgres-${var.env}"
  engine         = "postgres"
  engine_version = var.engine_version
  # Use new instance_class variable, fallback to old 'instance' for backwards compatibility
  instance_class = var.instance_class != "db.t4g.micro" ? var.instance_class : var.instance
  # Use new allocated_storage variable, fallback to old 'storage' for backwards compatibility
  allocated_storage      = var.allocated_storage != 20 ? var.allocated_storage : tonumber(var.storage)
  storage_type           = var.storage_type
  storage_encrypted      = var.storage_encrypted
  multi_az               = var.multi_az
  deletion_protection    = var.deletion_protection
  skip_final_snapshot    = var.skip_final_snapshot
  username               = local.db_username
  db_name                = local.db_name
  password                            = aws_ssm_parameter.postgres_password.value
  vpc_security_group_ids              = [aws_security_group.database.id]
  publicly_accessible                 = var.public_access
  iam_database_authentication_enabled = var.iam_database_authentication_enabled

  tags = {
    Name        = "${var.project}-postgres-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Aurora Serverless v2 Cluster (when aurora = true)
resource "aws_rds_cluster" "aurora" {
  count                  = var.aurora ? 1 : 0
  cluster_identifier     = "${var.project}-aurora-${var.env}"
  engine                 = "aurora-postgresql"
  engine_mode            = "provisioned"
  engine_version         = lookup(local.aurora_version_map, var.engine_version, "17.5")
  database_name          = local.db_name
  master_username        = local.db_username
  master_password        = aws_ssm_parameter.postgres_password.value
  skip_final_snapshot                 = true
  vpc_security_group_ids              = [aws_security_group.database.id]
  db_subnet_group_name                = aws_db_subnet_group.aurora[0].name
  iam_database_authentication_enabled = var.iam_database_authentication_enabled

  serverlessv2_scaling_configuration {
    min_capacity = var.min_capacity
    max_capacity = var.max_capacity
  }

  lifecycle {
    ignore_changes = [
      master_password,
      engine_version # Allow AWS to manage minor version updates automatically
    ]
  }

  tags = {
    Name        = "${var.project}-aurora-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Aurora Serverless v2 Instance
resource "aws_rds_cluster_instance" "aurora" {
  count                      = var.aurora ? 1 : 0
  identifier                 = "${var.project}-aurora-instance-${var.env}"
  cluster_identifier         = aws_rds_cluster.aurora[0].id
  instance_class             = "db.serverless"
  engine                     = aws_rds_cluster.aurora[0].engine
  engine_version             = aws_rds_cluster.aurora[0].engine_version
  publicly_accessible        = var.public_access
  auto_minor_version_upgrade = true # Always enable automatic minor version updates

  lifecycle {
    ignore_changes = [
      engine_version # Allow AWS to manage minor version updates
    ]
  }

  tags = {
    Name        = "${var.project}-aurora-instance-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# DB Subnet Group for Aurora
resource "aws_db_subnet_group" "aurora" {
  count      = var.aurora ? 1 : 0
  name       = "${var.project}-aurora-subnet-${var.env}"
  subnet_ids = var.subnet_ids

  tags = {
    Name        = "${var.project}-aurora-subnet-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}