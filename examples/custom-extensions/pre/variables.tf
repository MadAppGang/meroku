# ============================================================================
# VARIABLES FOR CUSTOM PRE-MODULE
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
