variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "workloads" {
  description = "Every workload of the environment, keyed by name, as the workload contract describes it. Only the task role name is read here. The backend is the entry keyed \"backend\"."
  type = map(object({
    task_role_name = string
  }))
}

variable "shared" {
  description = "Shared folders, shared/<name>/ in the bucket, with the workloads that may read and write them and the workloads that may only read them."
  type = list(object({
    name       = string
    read_write = optional(list(string), [])
    read       = optional(list(string), [])
  }))
  default = []

  validation {
    condition     = alltrue([for s in var.shared : can(regex("^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$", s.name))])
    error_message = "A shared folder name is 1 to 32 characters of lower-case letters, digits and hyphens, and starts and ends with a letter or a digit."
  }

  validation {
    condition     = length(distinct([for s in var.shared : s.name])) == length(var.shared)
    error_message = "Each shared folder name may appear only once."
  }
}

variable "backend_bucket_name" {
  description = "Name of the backend bucket. Every workload except the backend gets it as AWS_S3_BUCKET."
  type        = string
}

variable "backend_bucket_arn" {
  description = "ARN of the backend bucket. Workloads other than the backend get object-level read-write on it here; the backend keeps its own grant in modules/workloads."
  type        = string
}

variable "own_backend_bucket_env" {
  description = "Workloads whose YAML already sets AWS_S3_BUCKET. Their value is kept and this module does not add its own; their grant on the backend bucket is unchanged."
  type        = list(string)
  default     = []
}
