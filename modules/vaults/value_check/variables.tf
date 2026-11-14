variable "vault" {
  description = "The vault's name, for the warning."
  type        = string
}

variable "path" {
  description = "The vault's SSM path, /<env>/<project>/vault/<name>, without a trailing slash."
  type        = string
}

variable "names" {
  description = "The secret names the YAML declares for the vault."
  type        = list(string)
}
