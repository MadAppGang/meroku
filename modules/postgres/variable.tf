variable "env" {
  type    = string
  default = "dev"
}


variable "vpc_id" {
  type = string
}
variable "subnet_ids" {
  type = list(string)
}

variable "project" {
  type = string
}

variable "db_name" {
  type        = string
  default     = ""
  description = "Database name. If empty, uses project name as default"
}

variable "username" {
  type        = string
  default     = ""
  description = "Master username. If empty, defaults to 'postgres'"
}

variable "instance" {
  type        = string
  default     = "db.t3.micro"
  description = "DEPRECATED: Use instance_class instead. RDS instance type for standard (non-Aurora) deployment"
}

variable "storage" {
  type        = string
  default     = "20"
  description = "DEPRECATED: Use allocated_storage instead. Storage size in GB"
}

variable "public_access" {
  type    = bool
  default = false
}

variable "engine_version" {
  default     = "17"
  description = "PostgreSQL major version (13, 14, 15, 16, 17)"
}

variable "aurora" {
  type        = bool
  default     = false
  description = "Enable Aurora Serverless v2 instead of standard RDS"
}

variable "min_capacity" {
  type        = number
  default     = 0
  description = "Minimum capacity for Aurora Serverless v2 (in ACUs) - 0 allows pausing when idle"
}

variable "max_capacity" {
  type        = number
  default     = 1
  description = "Maximum capacity for Aurora Serverless v2 (in ACUs)"
}

# RDS-specific configuration (when aurora is false)
variable "instance_class" {
  type        = string
  default     = "db.t4g.micro"
  description = "RDS instance class (db.t4g.micro, db.m6i.large, etc.)"
}

variable "allocated_storage" {
  type        = number
  default     = 20
  description = "Allocated storage size in GB (20-65536)"
}

variable "storage_type" {
  type        = string
  default     = "gp3"
  description = "Storage type - gp3 (General Purpose SSD)"
}

variable "multi_az" {
  type        = bool
  default     = false
  description = "Enable Multi-AZ deployment for high availability"
}

variable "storage_encrypted" {
  type        = bool
  default     = true
  description = "Enable storage encryption at rest"
}

variable "deletion_protection" {
  type        = bool
  default     = false
  description = "Enable deletion protection to prevent accidental deletion"
}

variable "skip_final_snapshot" {
  type        = bool
  default     = true
  description = "Skip final snapshot when deleting (not recommended for production)"
}

variable "iam_database_authentication_enabled" {
  type        = bool
  default     = false
  description = "Enable IAM database authentication for passwordless access using IAM roles"
}

resource "random_password" "postgres" {
  length           = 16
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}


resource "aws_ssm_parameter" "postgres_password" {
  name  = "/${var.env}/${var.project}/postgres_password"
  type  = "SecureString"
  value = random_password.postgres.result

  tags = {
    Name        = "${var.project}-postgres-password-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

// propagade the result to backend env
resource "aws_ssm_parameter" "postgres_password_backend" {
  name  = "/${var.env}/${var.project}/backend/pg_database_password"
  type  = "SecureString"
  value = random_password.postgres.result

  tags = {
    Name        = "${var.project}-postgres-password-backend-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


