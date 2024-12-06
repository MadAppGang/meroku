variable "project" {
  type        = string
  description = "Project name"
}

variable "env" {
  type        = string
  description = "Environment name"
}


variable "vpc_id" {
  type        = string
  description = "VPC ID where to create EFS"
}

variable "private_subnets" {
  type        = list(string)
  description = "List of private subnet IDs"
}


