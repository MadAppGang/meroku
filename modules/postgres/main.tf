# Map user-friendly versions to Aurora-specific versions
locals {
  aurora_version_map = {
    "17" = "17.2"
    "16" = "16.4" 
    "15" = "15.6"
    "14" = "14.11"
    "13" = "13.14"
  }
}

# Standard RDS Instance (when aurora = false)
resource "aws_db_instance" "database" {
  count                  = var.aurora ? 0 : 1
  identifier             = "${var.project}-postgres-${var.env}"
  engine                 = "postgres"
  engine_version         = var.engine_version
  instance_class         = var.instance
  allocated_storage      = var.storage
  username               = var.username
  db_name                = var.db_name
  password               = aws_ssm_parameter.postgres_password.value
  skip_final_snapshot    = true
  vpc_security_group_ids = [aws_security_group.database.id]
  publicly_accessible    = var.public_access

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
  count                   = var.aurora ? 1 : 0
  cluster_identifier      = "${var.project}-aurora-${var.env}"
  engine                  = "aurora-postgresql"
  engine_mode             = "provisioned"
  engine_version          = lookup(local.aurora_version_map, var.engine_version, "16.4")
  database_name           = var.db_name
  master_username         = var.username
  master_password         = aws_ssm_parameter.postgres_password.value
  skip_final_snapshot     = true
  vpc_security_group_ids  = [aws_security_group.database.id]
  db_subnet_group_name    = aws_db_subnet_group.aurora[0].name
  
  serverlessv2_scaling_configuration {
    min_capacity = var.min_capacity
    max_capacity = var.max_capacity
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
  count              = var.aurora ? 1 : 0
  identifier         = "${var.project}-aurora-instance-${var.env}"
  cluster_identifier = aws_rds_cluster.aurora[0].id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.aurora[0].engine
  engine_version     = aws_rds_cluster.aurora[0].engine_version

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