variable "domain_name" {
  description = "The root domain name to create DNS zone for"
  type        = string
}

variable "trusted_account_ids" {
  description = "List of AWS account IDs that can assume the delegation role"
  type        = list(string)
  default     = []
}

variable "create_ns_records" {
  description = "Whether to create NS records for the root domain"
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}