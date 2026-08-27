data "aws_region" "current" {}

# Authorization is chosen per environment (var.auth_mode), not hardcoded.
#
# cognito and oidc are verified by AppSync itself: AWS fetches the provider's
# keys and checks the signature, issuer and audience before the request reaches a
# resolver, with no Lambda on the path. lambda deploys auth_lambda/ and is for
# providers those two cannot express.
resource "aws_appsync_graphql_api" "pubsub" {
  name                = "${var.project}-${var.env}-pubsub-api"
  authentication_type = local.authentication_type
  schema              = local.schema_content

  dynamic "lambda_authorizer_config" {
    for_each = local.use_lambda_auth ? [1] : []
    content {
      authorizer_uri = one(aws_lambda_function.function[*].arn)
    }
  }

  dynamic "user_pool_config" {
    for_each = local.use_cognito_auth ? [1] : []
    content {
      user_pool_id   = var.cognito_user_pool_id
      aws_region     = var.cognito_user_pool_region != "" ? var.cognito_user_pool_region : data.aws_region.current.name
      default_action = var.cognito_default_action
      # Null means every app client in the pool is accepted. modules/cognito
      # creates web, mobile and dashboard clients on one pool, so leaving this
      # unset lets a token minted for any of them reach this API — user pool mode
      # has no other audience check.
      app_id_client_regex = var.cognito_app_id_client_regex != "" ? var.cognito_app_id_client_regex : null
    }
  }

  dynamic "openid_connect_config" {
    for_each = local.use_oidc_auth ? [1] : []
    content {
      issuer = var.oidc_issuer
      # AppSync treats client_id as the audience check. Empty means "accept any
      # audience from this issuer", which is why the module nags about setting it.
      client_id = var.oidc_client_id != "" ? var.oidc_client_id : null
      auth_ttl  = var.oidc_auth_ttl_ms > 0 ? var.oidc_auth_ttl_ms : null
      iat_ttl   = var.oidc_iat_ttl_ms > 0 ? var.oidc_iat_ttl_ms : null
    }
  }

  # An API key is a second, weaker front door: it bypasses whatever auth_mode
  # verifies. It used to be attached unconditionally, so every deployment of this
  # module had one. Now nothing gets one without asking.
  dynamic "additional_authentication_provider" {
    for_each = var.api_key_enabled ? [1] : []
    content {
      authentication_type = "API_KEY"
    }
  }

  # Fail at plan time with a sentence the operator can act on, rather than at
  # apply time with an AWS API error, or - worse - succeeding with an
  # authorizer that trusts nothing and denies everyone.
  lifecycle {
    precondition {
      condition     = !local.use_cognito_auth || var.cognito_user_pool_id != ""
      error_message = "auth_mode = \"cognito\" requires cognito_user_pool_id. Enable modules/cognito in this environment and pass module.cognito.user_pool_id."
    }
    precondition {
      condition     = !local.use_oidc_auth || var.oidc_issuer != ""
      error_message = "auth_mode = \"oidc\" requires oidc_issuer, the https:// issuer URL of your identity provider. There is no default: whoever controls that URL can mint tokens this API accepts."
    }
    precondition {
      condition     = !local.use_lambda_auth || var.jwks_uri != ""
      error_message = "auth_mode = \"lambda\" requires jwks_uri, the https:// JWKS endpoint whose keys sign your tokens. There is no default: an unset value would make the authorizer deny every request."
    }
    # Believing a claim policy is enforced when nothing reads it is worse than
    # not having asked for one, so this refuses to plan rather than dropping it.
    precondition {
      condition     = length(var.required_claims) == 0 || local.use_lambda_auth
      error_message = "required_claims is only enforced when auth_mode = \"lambda\". AMAZON_COGNITO_USER_POOLS and OPENID_CONNECT verify the signature, the issuer and the audience, and cannot check any other claim. Either switch to lambda mode or drop required_claims."
    }
  }

  tags = {
    Name        = "${var.project}-${var.env}-pubsub-api"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# The expiry is derived from a timestamp held in state instead of from
# timestamp(), which was re-evaluated on every plan and so proposed a change on
# every apply. time_rotating rolls over halfway through the key's life, at which
# point Terraform extends the expiry in place (aws_appsync_api_key updates
# `expires` without issuing a new key).
resource "time_rotating" "api_key" {
  count = var.api_key_enabled ? 1 : 0

  rotation_days = max(1, floor(var.api_key_expiration_days / 2))
}

resource "aws_appsync_api_key" "pubsub" {
  count = var.api_key_enabled ? 1 : 0

  api_id      = aws_appsync_graphql_api.pubsub.id
  description = "${var.project}-${var.env} pubsub API key. Bypasses auth_mode = ${var.auth_mode}."
  expires     = timeadd(time_rotating.api_key[0].rfc3339, "${var.api_key_expiration_days * 24}h")
}

resource "aws_iam_role" "appsync" {
  name = module.naming.names["appsync_role"]

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "appsync.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name        = "${var.project}-${var.env}-appsync-role"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "appsync_logs" {
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSAppSyncPushToCloudWatchLogs"
  role       = aws_iam_role.appsync.name
}

resource "aws_appsync_datasource" "none" {
  api_id = aws_appsync_graphql_api.pubsub.id
  name   = "None"
  type   = "NONE"
}

# Create the AppSync resolvers
resource "aws_appsync_resolver" "resolvers" {
  for_each = local.vtl_templates != null ? merge([
    for type, fields in local.vtl_templates : {
      for field, templates in fields :
      "${type}.${field}" => {
        type     = type
        field    = field
        request  = try(templates.request, null)
        response = try(templates.response, null)
      } if templates != null
    } if fields != null
  ]...) : {}

  api_id      = aws_appsync_graphql_api.pubsub.id
  type        = each.value.type
  field       = each.value.field
  data_source = aws_appsync_datasource.none.name

  request_template  = each.value.request != null ? each.value.request : ""
  response_template = each.value.response != null ? each.value.response : ""
}
