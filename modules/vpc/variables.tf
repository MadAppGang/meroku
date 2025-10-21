variable "project" {
  description = "Project name"
  type        = string
}

variable "env" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for VPC (creates 2 AZs with public subnets only)"
  type        = string
  default     = "10.0.0.0/16"
}
