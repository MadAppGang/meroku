
resource "aws_service_discovery_service" "pgadmin" {
  count = var.pgadmin ? 1 : 0
  name  = "pgadmin" 
  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.local.id
    routing_policy = "MULTIVALUE"
    dns_records {
      ttl  = 10
      type = "A"
    }
  }
  health_check_custom_config {
    failure_threshold = 5
  }
}
