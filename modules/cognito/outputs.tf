# This module used to export nothing at all, so the user pool it creates could
# not be referenced by anything else in the environment — AppSync in particular
# had no way to use it and fell back to a Lambda authorizer plus an API key.
#
# None of these values are secrets. A user pool id, its ARN, its issuer endpoint
# and the public client ids are all handed to browsers and mobile apps as part of
# normal sign-in. The one secret this module can produce is the dashboard
# client's `client_secret` (that client sets `generate_secret = true`), and it is
# deliberately NOT exported: nothing in this repo consumes it, and an output is
# stored in plaintext in the Terraform state of every environment that reads it.

output "user_pool_id" {
  description = "Cognito user pool id, e.g. us-east-1_XXXXXXXXX. Pass to modules/appsync as cognito_user_pool_id."
  value       = aws_cognito_user_pool.user_pool.id
}

output "user_pool_arn" {
  description = "ARN of the user pool, for IAM policies that scope to it."
  value       = aws_cognito_user_pool.user_pool.arn
}

output "user_pool_endpoint" {
  description = <<-EOT
    Pool endpoint without a scheme, e.g. cognito-idp.<region>.amazonaws.com/<pool-id>.

    Prefix it with https:// to get the OIDC issuer for this pool, which is what
    the `iss` claim of its tokens contains and what an OIDC-aware client uses for
    discovery (https://<endpoint>/.well-known/openid-configuration).
  EOT
  value       = aws_cognito_user_pool.user_pool.endpoint
}

output "user_pool_issuer" {
  description = "OIDC issuer URL of this pool (https://<endpoint>). Matches the `iss` claim of its tokens."
  value       = "https://${aws_cognito_user_pool.user_pool.endpoint}"
}

output "user_pool_domain" {
  description = "Hosted UI domain prefix, or null when enable_user_pool_domain is false."
  value       = one(aws_cognito_user_pool_domain.cognito_domain[*].domain)
}

# Client ids are null when the matching client is disabled, rather than failing
# to plan. `one()` on a count'd resource returns null for an empty list.

output "web_client_id" {
  description = "App client id for the web client, or null when enable_web_client is false."
  value       = one(aws_cognito_user_pool_client.web[*].id)
}

output "mobile_client_id" {
  description = "App client id for the mobile client, or null when enable_mobile_client is false."
  value       = one(aws_cognito_user_pool_client.mobile[*].id)
}

output "dashboard_client_id" {
  description = "App client id for the dashboard client, or null when enable_dashboard_client is false."
  value       = one(aws_cognito_user_pool_client.dashboard[*].id)
}
