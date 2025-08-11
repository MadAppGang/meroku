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
  type = string
  default = ""
  description = "Database name. If empty, uses project name as default"
}

variable "username" {
  type    = string
  default = ""
  description = "Master username. If empty, defaults to 'postgres'"
}

variable "instance" {
  type    = string
  default = "db.t3.micro"
}

variable "storage" {
  type    = string
  default = "20"
}

variable "public_access" {
  type    = bool
  default = false
}

variable "engine_version" {
  default = "16"
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


