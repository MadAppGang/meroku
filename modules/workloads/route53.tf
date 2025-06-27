resource "aws_service_discovery_private_dns_namespace" "local" {
  name = var.private_dns_name
  vpc  = var.vpc_id

  tags = {
    Name        = var.private_dns_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}



# dev.demo.quadrolith.com