variable "env" {
  type    = string
  default = "dev"
}

variable "project" {
  type = string
}

variable "username" {
  type = string
  default = "postgres"
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

resource "aws_ssm_parameter" "postgres_password" {
  name = "/${var.env}/${var.project}/postgres_password"
  type = "SecureString"
  value = random_password.postgres.result
}

