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


variable "pgadmin_enabled" {
  type    = bool
  default = "false"
}
variable "pgadmin_email" {
  type    = string
  default = "admin@madappgang.com"
}

variable "project" {
  type = string
}

variable "db_name" {
  type = string
}

variable "username" {
  type    = string
  default = "postgres"
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
  default = "14"
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

resource "random_password" "pgadmin" {
  length           = 8
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}



resource "aws_ssm_parameter" "pgadmin_password" {
  count = var.pgadmin_enabled ? 1 : 0
  name  = "/${var.env}/${var.project}/pgadmin_password"
  type  = "SecureString"
  value = random_password.pgadmin.result

  tags = {
    Name        = "${var.project}-pgadmin-password-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

