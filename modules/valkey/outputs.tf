output "address" {
  description = "Host name of the cache's endpoint."
  value       = aws_elasticache_serverless_cache.valkey.endpoint[0].address
}

output "port" {
  description = "Port of the cache's endpoint."
  value       = aws_elasticache_serverless_cache.valkey.endpoint[0].port
}

output "security_group_id" {
  description = "The cache's own security group. Custom Terraform may add ingress rules to it for clients meroku does not manage."
  value       = aws_security_group.valkey.id
}

output "ingress_keys" {
  description = "The sg_key of every security group allowed in, one rule pair each."
  value       = sort(keys(local.access_groups))
}

# Every workload gets an entry, so the root can index the map by any workload
# name; only the workloads in var.access get variables. The cache needs no
# password, so there are no secrets.
output "environment" {
  description = "Environment variables for each workload, keyed by workload name."
  value = { for w in keys(var.workloads) : w => contains(var.access, w) ? [
    { name = "VALKEY_HOST", value = aws_elasticache_serverless_cache.valkey.endpoint[0].address },
    { name = "VALKEY_PORT", value = tostring(aws_elasticache_serverless_cache.valkey.endpoint[0].port) },
    { name = "VALKEY_URL", value = "rediss://${aws_elasticache_serverless_cache.valkey.endpoint[0].address}:${aws_elasticache_serverless_cache.valkey.endpoint[0].port}" },
  ] : [] }
}
