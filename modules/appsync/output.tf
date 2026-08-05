output "api_url" {
  description = "GraphQL endpoint of the API."
  value       = aws_appsync_graphql_api.pubsub.uris["GRAPHQL"]
}

output "api_id" {
  description = "AppSync API id."
  value       = aws_appsync_graphql_api.pubsub.id
}

output "auth_mode" {
  description = "Authorization mode actually deployed: cognito, oidc or lambda."
  value       = var.auth_mode
}

# Null unless api_key_enabled is true. This used to reference a key that was
# always created; now the key is opt-in, so the output has to tolerate its
# absence rather than fail to plan.
#
# Anything reading this must handle null - and should ask itself why it wants an
# API key at all, since holding one skips the auth_mode checks entirely.
output "api_key" {
  description = "API key value when api_key_enabled is true, otherwise null."
  value       = one(aws_appsync_api_key.pubsub[*].key)
  sensitive   = true
}
