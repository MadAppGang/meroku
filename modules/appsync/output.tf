output "api_url" {
  value = aws_appsync_graphql_api.pubsub.uris["GRAPHQL"]
}

output "api_key" {
  value     = aws_appsync_api_key.pubsub.key
  sensitive = true
}

