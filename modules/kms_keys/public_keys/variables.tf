variable "key_arns" {
  description = "The ARN of every asymmetric signing key, never an HMAC key, keyed by key name. A key not yet created has an unknown ARN; its public key is read during apply."
  type        = map(string)
  default     = {}
  nullable    = false
}
