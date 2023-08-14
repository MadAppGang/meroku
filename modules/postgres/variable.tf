variable "env" {
  type    = string
  default = "dev"
}


variable "vpc_id" {
  type = string
}


variable "project" {
  type = string
}

variable "db_name" {
  type = string
}

variable "username" {
  type = string
  default = "postgres"
}

variable "pgadmin" {
  type = bool
  default = "false"
}
variable "pgadmin_email" {
  type = string
  default = "admin@madappgang.com"
}

variable "instance" {
  type = string
  default = "db.t3.micro"
}

variable "storage" {
  type = string
  default = "20"
}

resource "random_password" "postgres" {
  length           = 16
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

resource "random_password" "pgadmin" {
  length           = 8
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}

resource "aws_ssm_parameter" "postgres_password" {
  name = "/${var.env}/${var.project}/postgres_password"
  type = "SecureString"
  value = random_password.postgres.result
}

// propagade the result to backend env
resource "aws_ssm_parameter" "postgres_password_backend" {
  name = "/${var.env}/${var.project}/backend/pg_database_password"
  type = "SecureString"
  value = random_password.postgres.result
}


resource "aws_ssm_parameter" "pgadmin_password" {
  count = var.pgadmin ? 1 : 0
  name = "/${var.env}/${var.project}/pgadmin_password"
  type = "SecureString"
  value = random_password.pgadmin.result
}