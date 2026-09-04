variable "pool_names" {
  description = <<-EOT
    Names of the ENABLED compute pools, which is keys(local.pools) in the
    caller. A disabled pool must not appear here: a workload pointed at one has
    to fail, and the message is what tells the reader to enable it.
  EOT
  type        = list(string)
  default     = []
}

variable "workloads" {
  description = <<-EOT
    The workloads to check, keyed however the caller keys them. `pool` is the
    pool the workload asked for, or null when it runs on Fargate, which is the
    same null modules/workloads uses everywhere to mean "not on a pool".

    `subject` and `noun` carry the two halves of the sentence that differ
    between a service and the backend: subject opens it ("Service \"api\"",
    "The backend") and noun appears in the closing clause ("point the service
    at", "point the backend at").
  EOT
  type = map(object({
    subject = string
    noun    = string
    pool    = optional(string)
  }))
  default = {}
}
