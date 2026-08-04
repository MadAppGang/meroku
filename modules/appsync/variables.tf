variable "project" {
  type = string
}

variable "env" {
  type = string
}

variable "schema_file" {
  type    = string
  default = ""
}

variable "vtl_templates_yaml" {
  type    = string
  default = ""
}


variable "auth_lambda_path" {
  type    = string
  default = ""
}

# --- Lambda authorizer configuration -----------------------------------------
# The authorizer has no built-in identity provider. Everything it trusts comes
# from here, so that enabling AppSync auth is always a deliberate choice.

variable "jwks_uri" {
  description = <<-EOT
    Required. HTTPS URL of the JWKS document holding the public keys that sign
    your JWTs (for example https://<your-idp-host>/.well-known/jwks.json).
    This is the only thing the authorizer trusts: whoever controls this endpoint
    can mint tokens your API accepts, so point it at an identity provider you
    own. There is deliberately no default - an unset value makes the authorizer
    deny every request.
  EOT
  type        = string
  nullable    = false

  validation {
    condition     = can(regex("^https://[^ ]+$", var.jwks_uri))
    error_message = "jwks_uri must be an https:// URL. Plaintext http would let a network attacker swap the signing keys."
  }
}

variable "jwt_issuer" {
  description = <<-EOT
    Optional. Expected `iss` claim. When set, tokens from any other issuer are
    rejected even if the signature checks out. Accepts a comma-separated list.
    Strongly recommended when the JWKS endpoint serves more than one issuer.
  EOT
  type        = string
  default     = ""
  nullable    = false
}

variable "jwt_audience" {
  description = <<-EOT
    Optional. Expected `aud` claim. When set, tokens minted for a different
    audience are rejected even if the signature checks out. Accepts a
    comma-separated list. Strongly recommended when the same identity provider
    issues tokens for more than one API.
  EOT
  type        = string
  default     = ""
  nullable    = false
}


locals {
  vtl_templates  = var.vtl_templates_yaml != "" ? yamldecode(file(var.vtl_templates_yaml)) : yamldecode(file("${path.module}/vtl_templates.yaml"))
  schema_content = var.schema_file != "" ? file(var.schema_file) : file("${path.module}/schema.graphql")
  auth_lambda    = var.auth_lambda_path != "" ? var.auth_lambda_path : "${path.module}/auth_lambda"
}
