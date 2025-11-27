# ============================================================================
# VARIABLES FOR CUSTOM POST-MODULE
# These are automatically passed by meroku when the module is invoked
# ============================================================================

# Project context
variable "project" {
  description = "Project name from YAML config"
  type        = string
}

variable "env" {
  description = "Environment name (dev, staging, prod)"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
}

variable "account_id" {
  description = "AWS account ID"
  type        = string
}

# VPC context
variable "vpc_id" {
  description = "VPC ID where resources are deployed"
  type        = string
}

variable "subnet_ids" {
  description = "List of subnet IDs"
  type        = list(string)
}

# Core module outputs - Workloads
variable "api_endpoint" {
  description = "API Gateway endpoint URL with stage"
  type        = string
}

variable "api_gateway_id" {
  description = "API Gateway ID"
  type        = string
}

variable "ecs_cluster_arn" {
  description = "ECS cluster ARN"
  type        = string
}

variable "ecs_cluster_name" {
  description = "ECS cluster name"
  type        = string
}

variable "backend_ecr_repo_url" {
  description = "Backend ECR repository URL"
  type        = string
}

variable "backend_task_role" {
  description = "Backend ECS task IAM role name"
  type        = string
}

# Core module outputs - Domain (optional, may be empty)
variable "domain_zone_id" {
  description = "Route53 hosted zone ID (empty if domain not enabled)"
  type        = string
  default     = ""
}

variable "api_domain" {
  description = "Custom API domain name (empty if domain not enabled)"
  type        = string
  default     = ""
}

# Core module outputs - Database (optional, may be empty)
variable "db_endpoint" {
  description = "Database endpoint (empty if postgres not enabled)"
  type        = string
  default     = ""
}
