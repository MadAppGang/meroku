variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "vpc_id" {
  description = "VPC the cache's endpoint and security group live in."
  type        = string
}

variable "subnet_ids" {
  description = "Subnets the cache's VPC endpoint is placed in."
  type        = list(string)
}

variable "engine_version" {
  description = "Valkey major version."
  type        = string
  default     = "8"

  validation {
    condition     = contains(["7", "8", "9"], var.engine_version)
    error_message = "engine_version must be \"7\", \"8\" or \"9\"."
  }
}

variable "max_storage_gb" {
  description = "Upper limit on the data the cache may hold, in GB. It caps the storage bill; there is never a minimum, which would raise the monthly floor."
  type        = number
  default     = 1

  validation {
    condition     = var.max_storage_gb >= 1 && floor(var.max_storage_gb) == var.max_storage_gb
    error_message = "max_storage_gb must be a whole number of at least 1."
  }
}

variable "workloads" {
  description = "Every workload of the environment, keyed by name, as the workload contract describes it. Only the security group and its plan-known key are read here."
  type = map(object({
    security_group_id = string
    sg_key            = string
  }))
}

variable "access" {
  description = "Names of the workloads that may connect. Each one's security group may reach the cache on 6379 and 6380, and its container gets VALKEY_HOST, VALKEY_PORT and VALKEY_URL."
  type        = list(string)
  default     = []

  validation {
    condition     = length(distinct(var.access)) == length(var.access)
    error_message = "A workload may appear in access only once."
  }

  validation {
    condition     = alltrue([for w in var.access : contains(keys(var.workloads), w)])
    error_message = "Every name in access must be a workload of the environment."
  }
}
