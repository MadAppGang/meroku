resource "aws_service_discovery_private_dns_namespace" "local" {
  name = var.private_dns_name
  vpc  = var.vpc_id
}



# dev.demo.quadrolith.com