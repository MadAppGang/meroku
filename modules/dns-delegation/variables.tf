variable "subdomain" {
  description = "The subdomain to create and delegate"
  type        = string
}

variable "root_domain" {
  description = "The root domain name"
  type        = string
}

variable "root_zone_id" {
  description = "The zone ID of the root domain"
  type        = string
}

variable "environment" {
  description = "The environment name (e.g., dev, staging, prod)"
  type        = string
}

variable "root_account_id" {
  description = "The AWS account ID that owns the root zone"
  type        = string
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}