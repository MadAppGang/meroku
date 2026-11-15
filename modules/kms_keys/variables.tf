variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "workloads" {
  description = "Every workload of the environment, keyed by name, as the workload contract describes it. Only the task role name is read here."
  type = map(object({
    task_role_name = string
  }))
}

variable "keys" {
  description = "Every KMS signing key of the environment, retired ones included, keyed by name. key_spec is the key type; AWS cannot change it on an existing key."
  type = map(object({
    key_spec = optional(string, "ECC_NIST_P256")
  }))
  default  = {}
  nullable = false

  validation {
    condition     = alltrue([for k, _ in var.keys : can(regex("^[a-z][a-z0-9_]{0,30}$", k))])
    error_message = "A kms key name is 1 to 31 lower-case letters, digits and underscores, starting with a letter."
  }

  validation {
    condition = alltrue([for k, v in var.keys : contains([
      "ECC_NIST_P256", "ECC_NIST_P384", "ECC_NIST_P521", "ECC_SECG_P256K1",
      "RSA_2048", "RSA_3072", "RSA_4096",
    ], v.key_spec)])
    error_message = "A kms key's key_spec is one of ECC_NIST_P256, ECC_NIST_P384, ECC_NIST_P521, ECC_SECG_P256K1, RSA_2048, RSA_3072, RSA_4096."
  }
}

variable "connections" {
  description = "One entry per workload and key it is connected to. access is sign, verify or both."
  type = list(object({
    workload = string
    key      = string
    access   = string
  }))
  default  = []
  nullable = false

  validation {
    condition     = alltrue([for c in var.connections : contains(["sign", "verify", "both"], c.access)])
    error_message = "A kms key connection's access is sign, verify or both."
  }

  validation {
    condition     = alltrue([for c in var.connections : contains(keys(var.keys), c.key)])
    error_message = "Every kms key connection names a key of var.keys."
  }

  validation {
    condition     = alltrue([for c in var.connections : contains(keys(var.workloads), c.workload)])
    error_message = "Every kms key connection names a workload of var.workloads."
  }

  validation {
    condition     = length(distinct([for c in var.connections : "${c.workload}/${c.key}"])) == length(var.connections)
    error_message = "A workload may be connected to each kms key only once."
  }
}
