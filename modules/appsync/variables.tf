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
  description = "Path to a project-supplied Lambda authorizer source tree. Only used when auth_mode = \"lambda\"; the bundled authorizer is used when empty."
  type        = string
  default     = ""
}

# --- Authorization mode ------------------------------------------------------
# This module used to hardcode AWS_LAMBDA and unconditionally attach an API key,
# which meant every deployment paid for a Lambda on the request path AND handed
# out a bearer credential that skipped it. Both are now choices.

variable "auth_mode" {
  description = <<-EOT
    How AppSync authenticates callers. Exactly one of:

      "cognito" - AMAZON_COGNITO_USER_POOLS. AWS validates the token against the
                  Cognito user pool given in cognito_user_pool_id. No Lambda
                  runs, so there is nothing to cold-start, pay for or maintain.
                  The natural choice when this environment already has a Cognito
                  user pool.

      "oidc"    - OPENID_CONNECT. AWS validates the token against the issuer
                  given in oidc_issuer, fetching that issuer's OIDC discovery
                  document and JWKS itself. Use this for any external identity
                  provider that publishes a standard discovery document. Also no
                  Lambda.

      "lambda"  - AWS_LAMBDA. Deploys the authorizer in auth_lambda/, which
                  verifies RS256 JWTs against jwks_uri. Use only when neither
                  native mode can express your provider, because it adds a
                  Lambda invocation (and a cold start) to uncached requests.

    Defaults to "lambda" because that is what this module did before the mode
    existed; upgrading without setting it does not change behaviour.
  EOT
  type        = string
  default     = "lambda"
  nullable    = false

  validation {
    condition     = contains(["cognito", "oidc", "lambda"], var.auth_mode)
    error_message = "auth_mode must be \"cognito\", \"oidc\" or \"lambda\"."
  }
}

# --- Cognito user pool mode --------------------------------------------------

variable "cognito_user_pool_id" {
  description = <<-EOT
    Required when auth_mode = "cognito". Id of the Cognito user pool whose access
    tokens this API accepts, e.g. us-east-1_XXXXXXXXX. modules/cognito exports it
    as `user_pool_id`.
  EOT
  type        = string
  default     = ""
  nullable    = false
}

variable "cognito_user_pool_region" {
  description = "Region the Cognito user pool lives in. Defaults to this module's region, which is correct unless the pool is in another region."
  type        = string
  default     = ""
  nullable    = false
}

variable "cognito_app_id_client_regex" {
  description = <<-EOT
    Which app clients of the pool this API accepts tokens from. AWS: "a regular
    expression for validating the incoming Amazon Cognito user pool app client
    ID. If this value isn't set, no filtering is applied."

    WHEN UNSET (the default) EVERY APP CLIENT IN THE POOL IS ACCEPTED. That is
    rarely what anyone means: modules/cognito creates web, mobile and dashboard
    clients on one pool, so an API meant for the dashboard also accepts a token
    minted for the mobile app. User pool mode has no separate audience field —
    this IS the audience check.

    Which claim it is matched against depends on the token the client sends:

      ID token     - the app client id is in `aud`
      access token - the app client id is in `client_id`

    That difference decides which token your clients should present, so pick one
    and be consistent; a client sending the wrong token type is filtered out even
    with a correct value here.

    Several clients are expressed as a pipe-separated alternation, the form AWS
    documents:

      "1F4G9H"                  # one client
      "1F4G9H|1J6L4B|6GS5MG"    # three clients, and no others

    The value is passed through untouched: AppSync applies its own regex
    semantics and meroku neither validates nor rebuilds it.
  EOT
  type        = string
  default     = ""
  nullable    = false
}

variable "cognito_default_action" {
  description = <<-EOT
    What AppSync does with a request that carries a valid user pool token but is
    not matched by an @aws_auth / @aws_cognito_user_pools directive on the field.

    "ALLOW" (default) means a valid token from the pool is enough. "DENY" means
    every field must name the groups allowed to reach it, which only works if
    your schema actually carries those directives — set it on a schema that has
    none and the API rejects everything.
  EOT
  type        = string
  default     = "ALLOW"
  nullable    = false

  validation {
    condition     = contains(["ALLOW", "DENY"], var.cognito_default_action)
    error_message = "cognito_default_action must be \"ALLOW\" or \"DENY\"."
  }
}

# --- OIDC mode ---------------------------------------------------------------

variable "oidc_issuer" {
  description = <<-EOT
    Required when auth_mode = "oidc". Issuer URL of the identity provider.
    AppSync appends /.well-known/openid-configuration to discover the provider's
    JWKS, so the provider has to publish one.

    There is deliberately no default and no built-in provider: whoever controls
    this URL can mint tokens your API accepts.

    KNOW THIS ABOUT SINGLE-MODE OIDC. AWS: "If an API is configured with multiple
    authorization types, AWS AppSync validates the issuer (iss claim) present in
    the JWT token from request headers by comparing it against the issuer URL
    specified in the API configuration. However, when an API is configured with
    only OPENID_CONNECT authorization, AWS AppSync skips this issuer URL
    validation step."

    So on an OIDC-only API the `iss` claim is not compared against this value.
    What still holds: AppSync fetches the signing keys from THIS issuer's
    discovery document, so a token signed by an unrelated provider fails
    signature verification and is rejected. What does not hold: `iss` itself is
    not asserted, so a token minted by this same issuer but carrying a different
    `iss` is not rejected on that basis.

    Do not "fix" this by attaching a second authorization type to trip the
    multi-mode path — adding an API key to gain a stricter issuer check would
    trade a narrow gap for an unauthenticated bypass, which is the mistake this
    module already made once. If your threat model needs `iss` asserted, use
    auth_mode = "lambda": that authorizer enforces jwt_issuer directly.
  EOT
  type        = string
  default     = ""
  nullable    = false

  validation {
    condition     = var.oidc_issuer == "" || can(regex("^https://[^ ]+$", var.oidc_issuer))
    error_message = "oidc_issuer must be an https:// URL. Over plaintext http a network attacker could swap the discovery document and the signing keys."
  }
}

variable "oidc_client_id" {
  description = <<-EOT
    Optional. The client identifier of the relying party at the OIDC provider —
    the value you were given when you registered this application. AppSync has no
    separate audience field, so this IS the audience check.

    It is matched against the token's `aud` claim, falling back to `azp` when
    `aud` is absent. That is the claim your identity provider must be configured
    to put this identifier in; if tokens arrive with it somewhere else, this
    check cannot see it.

    When unset, any audience from that issuer is accepted.

    Several client identifiers are expressed as a pipe-separated alternation, the
    form AWS documents ("You can specify a regular expression so that AWS AppSync
    can validate against multiple client identifiers at a time"):

      "1F4G9H"                  # one client
      "1F4G9H|1J6L4B|6GS5MG"    # three clients

    Needing more than one audience is therefore NOT a reason to reach for
    auth_mode = "lambda". Needing to check a claim other than iss/aud is.
  EOT
  type        = string
  default     = ""
  nullable    = false
}

variable "oidc_auth_ttl_ms" {
  description = <<-EOT
    Optional. Milliseconds after `auth_time` that a token remains acceptable.

    This bounds the `auth_time` claim — how long ago the user actually
    authenticated — not the token's expiry. `auth_time` is OPTIONAL in an OIDC
    token, so this setting does nothing unless your provider emits it. Use it to
    require a recent login rather than merely an unexpired token.

    0 (default) applies no additional bound.
  EOT
  type        = number
  default     = 0
  nullable    = false
}

variable "oidc_iat_ttl_ms" {
  description = <<-EOT
    Optional. Milliseconds after `iat` that a token remains acceptable.

    This bounds the `iat` claim — when the token was issued. AppSync requires
    tokens to include `iat`, so unlike auth_ttl this always applies once set. It
    caps a token's usable life independently of the `exp` its issuer chose.

    0 (default) applies no additional bound.
  EOT
  type        = number
  default     = 0
  nullable    = false
}

# --- Lambda authorizer mode --------------------------------------------------
# The authorizer has no built-in identity provider. Everything it trusts comes
# from here, so that enabling AppSync auth is always a deliberate choice.

variable "jwks_uri" {
  description = <<-EOT
    Required when auth_mode = "lambda". HTTPS URL of the JWKS document holding
    the public keys that sign your JWTs (for example
    https://<your-idp-host>/.well-known/jwks.json). This is the only thing the
    authorizer trusts: whoever controls this endpoint can mint tokens your API
    accepts, so point it at an identity provider you own. There is deliberately
    no default - an unset value makes the authorizer deny every request, and in
    lambda mode a precondition refuses to plan at all.
  EOT
  type        = string
  default     = ""
  nullable    = false

  validation {
    condition     = var.jwks_uri == "" || can(regex("^https://[^ ]+$", var.jwks_uri))
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

variable "required_claims" {
  description = <<-EOT
    Claims a verified token must carry to be accepted, checked after the
    signature, issuer and audience.

    Claim name -> list of accepted values. An empty list means "the claim must be
    present"; a non-empty list means "the claim must hold one of these values".
    A claim holding an array (group membership, a multi-valued aud) matches when
    ANY of its entries is accepted.

      required_claims = {
        tenant_id = []                 # must be present
        role      = ["admin", "ops"]   # must be one of these
      }

    This is the capability that selects auth_mode = "lambda". Neither
    AMAZON_COGNITO_USER_POOLS nor OPENID_CONNECT can check a claim beyond issuer
    and audience, so a policy that turns on `role` or `tenant_id` cannot be
    expressed natively — while merely accepting several audiences can (see
    oidc_client_id and cognito_app_id_client_regex, both of which take a
    pipe-separated list).

    This is for POLICY claims - `role`, `scope`, `tenant_id`, a plan tier. It is
    not the place to check WHO the caller is: `sub` identifies an individual, so
    there is no fixed value to list here, and a deployment-wide allowlist of user
    ids is not authorization. Per-user decisions belong in your resolvers, which
    read `sub` from `resolverContext` — the authorizer already puts it there.

    Only read in lambda mode. A precondition refuses to plan if it is set in a
    mode that would silently ignore it, because believing a claim is enforced
    when it is not is worse than not asking for it.

    Values are compared as strings. The authorizer denies with
    reason = "claim_denied" (distinct from "invalid_token" and from
    "authorizer_internal_error"), and denies everything if this policy is
    malformed rather than falling through to "no requirements".
  EOT
  type        = map(list(string))
  default     = {}
  nullable    = false

  validation {
    condition     = alltrue([for claim, _ in var.required_claims : trimspace(claim) != ""])
    error_message = "required_claims must not contain an empty claim name."
  }
}

# --- API key -----------------------------------------------------------------

variable "api_key_enabled" {
  description = <<-EOT
    Whether to attach an API_KEY additional authentication provider.

    Defaults to false, and that default is the point. An API key is an
    unattributable bearer credential that BYPASSES auth_mode entirely: a caller
    holding it reaches every resolver without presenting a JWT, so none of the
    issuer, audience or signature checks configured above apply to it. This
    module used to create one unconditionally and export it, which made the
    authorizer decorative.

    Enable it only for an API that is genuinely public, or for the length of a
    migration, and shorten api_key_expiration_days when you do.
  EOT
  type        = bool
  default     = false
  nullable    = false
}

variable "api_key_expiration_days" {
  description = <<-EOT
    How long an issued API key stays valid. AWS caps this at 365 days.

    The expiry used to be timeadd(timestamp(), "8760h"), which re-evaluated on
    every plan and so produced a change on every single apply - permanent drift
    that trained everyone to ignore the diff. It is now derived from a rotation
    timestamp held in state, so the value is stable between applies and Terraform
    moves it forward once it is halfway through its life.
  EOT
  type        = number
  default     = 365
  nullable    = false

  validation {
    condition     = var.api_key_expiration_days >= 1 && var.api_key_expiration_days <= 365
    error_message = "api_key_expiration_days must be between 1 and 365 (the AWS AppSync maximum)."
  }
}


locals {
  vtl_templates  = var.vtl_templates_yaml != "" ? yamldecode(file(var.vtl_templates_yaml)) : yamldecode(file("${path.module}/vtl_templates.yaml"))
  schema_content = var.schema_file != "" ? file(var.schema_file) : file("${path.module}/schema.graphql")
  auth_lambda    = var.auth_lambda_path != "" ? var.auth_lambda_path : "${path.module}/auth_lambda"

  use_cognito_auth = var.auth_mode == "cognito"
  use_oidc_auth    = var.auth_mode == "oidc"
  use_lambda_auth  = var.auth_mode == "lambda"

  authentication_type = {
    cognito = "AMAZON_COGNITO_USER_POOLS"
    oidc    = "OPENID_CONNECT"
    lambda  = "AWS_LAMBDA"
  }[var.auth_mode]
}
