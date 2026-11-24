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

  db_subnet_ids = var.private_subnets ? var.private_subnet_ids : var.subnet_ids

  # The subnet group names the configuration asks for. Strings, not the
  # resources' .name: the postconditions below need a value known at plan, so
  # they fail before anything is applied.
  #
  # null: the instance uses the account's default subnet group.
  instance_subnet_group = var.default_vpc ? null : module.naming.names[var.private_subnets ? "postgres_private_subnet" : "postgres_subnet"]
  aurora_subnet_group   = module.naming.names[var.private_subnets ? "aurora_private_subnet" : "aurora_subnet"]
}

# Standard RDS Instance (when aurora = false)
resource "aws_db_instance" "database" {
  count          = var.aurora ? 0 : 1
  identifier     = module.naming.names["postgres"]
  engine         = "postgres"
  engine_version = var.engine_version
  # Use new instance_class variable, fallback to old 'instance' for backwards compatibility
  instance_class = var.instance_class != "db.t4g.micro" ? var.instance_class : var.instance
  # Use new allocated_storage variable, fallback to old 'storage' for backwards compatibility
  allocated_storage                   = var.allocated_storage != 20 ? var.allocated_storage : tonumber(var.storage)
  storage_type                        = var.storage_type
  storage_encrypted                   = var.storage_encrypted
  multi_az                            = var.multi_az
  deletion_protection                 = var.deletion_protection
  skip_final_snapshot                 = var.skip_final_snapshot
  username                            = local.db_username
  db_name                             = local.db_name
  password                            = aws_ssm_parameter.postgres_password.value
  vpc_security_group_ids              = [aws_security_group.database.id]
  db_subnet_group_name                = one(aws_db_subnet_group.database[*].name)
  publicly_accessible                 = var.public_access
  iam_database_authentication_enabled = var.iam_database_authentication_enabled

  # AWS cannot move a database between subnet groups of one VPC. ignore_changes
  # keeps the group in state, and the postcondition refuses a configuration that
  # names another one.
  lifecycle {
    ignore_changes = [db_subnet_group_name]

    precondition {
      condition     = !(var.private_subnets && var.default_vpc)
      error_message = "postgres.private_subnets cannot be used in the AWS default VPC, which has no private subnets."
    }

    postcondition {
      # In the default VPC any group but this module's own two is accepted: the
      # plan deletes those, and an existing instance would still be using one.
      condition = local.instance_subnet_group == null ? (
        self.db_subnet_group_name != module.naming.names["postgres_subnet"] &&
        self.db_subnet_group_name != module.naming.names["postgres_private_subnet"]
      ) : self.db_subnet_group_name == local.instance_subnet_group
      error_message = "This database already exists in subnet group ${coalesce(self.db_subnet_group_name, "(none)")}, but the configuration places it in ${coalesce(local.instance_subnet_group, "the account's default subnet group")}. An existing database cannot move to another subnet group. Set whichever of postgres.private_subnets and use_default_vpc you changed back to its previous value."
    }
  }

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
  count                               = var.aurora ? 1 : 0
  cluster_identifier                  = module.naming.names["aurora"]
  engine                              = "aurora-postgresql"
  engine_mode                         = "provisioned"
  engine_version                      = lookup(local.aurora_version_map, var.engine_version, "17.5")
  database_name                       = local.db_name
  master_username                     = local.db_username
  master_password                     = aws_ssm_parameter.postgres_password.value
  skip_final_snapshot                 = true
  vpc_security_group_ids              = [aws_security_group.database.id]
  db_subnet_group_name                = aws_db_subnet_group.aurora[0].name
  iam_database_authentication_enabled = var.iam_database_authentication_enabled

  serverlessv2_scaling_configuration {
    min_capacity = var.min_capacity
    max_capacity = var.max_capacity
  }

  # db_subnet_group_name: see aws_db_instance.database.
  lifecycle {
    ignore_changes = [
      master_password,
      engine_version, # Allow AWS to manage minor version updates automatically
      db_subnet_group_name
    ]

    postcondition {
      condition     = self.db_subnet_group_name == local.aurora_subnet_group
      error_message = "This database already exists in subnet group ${coalesce(self.db_subnet_group_name, "(none)")}, but the configuration places it in ${local.aurora_subnet_group}. An existing database cannot move to another subnet group. Set postgres.private_subnets back to the previous value."
    }
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
  identifier                 = module.naming.names["aurora_instance"]
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

# DB Subnet Group for the standard instance. Without one RDS uses the account's
# default VPC, which is right only when the environment runs there too.
resource "aws_db_subnet_group" "database" {
  count      = !var.aurora && !var.default_vpc ? 1 : 0
  name       = local.instance_subnet_group
  subnet_ids = local.db_subnet_ids

  lifecycle {
    precondition {
      condition     = !var.private_subnets || length(var.private_subnet_ids) >= 2
      error_message = "postgres.private_subnets needs the VPC's private subnets in two Availability Zones, which exist only with a NAT egress_strategy."
    }
    precondition {
      condition     = length(module.naming.collisions) == 0
      error_message = "Two resources in modules/postgres would get the same name: ${join(", ", module.naming.collisions)}."
    }
  }

  tags = {
    Name        = local.instance_subnet_group
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# DB Subnet Group for Aurora
resource "aws_db_subnet_group" "aurora" {
  count      = var.aurora ? 1 : 0
  name       = local.aurora_subnet_group
  subnet_ids = local.db_subnet_ids

  lifecycle {
    precondition {
      condition     = !(var.private_subnets && var.default_vpc)
      error_message = "postgres.private_subnets cannot be used in the AWS default VPC, which has no private subnets."
    }
    precondition {
      condition     = !var.private_subnets || length(var.private_subnet_ids) >= 2
      error_message = "postgres.private_subnets needs the VPC's private subnets in two Availability Zones, which exist only with a NAT egress_strategy."
    }
    precondition {
      condition     = length(module.naming.collisions) == 0
      error_message = "Two resources in modules/postgres would get the same name: ${join(", ", module.naming.collisions)}."
    }
  }

  tags = {
    Name        = local.aurora_subnet_group
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}
