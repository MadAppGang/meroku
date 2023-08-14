resource "aws_service_discovery_private_dns_namespace" "local" {
  name = var.private_dns_name 
  vpc  = var.vpc_id
}



resource "aws_service_discovery_service" "backend" {
  name =  "backend" 
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

resource "aws_service_discovery_service" "mockoon" {
  count = var.env == "dev" ? 1 : 0
  name  = "mockoon" 
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


resource "aws_route53_record" "api" {
  zone_id = var.zone_id
  name = "api.${var.env == "prod" ? "app" : var.env}.${var.domain}"
  type = "CNAME"
  ttl = 60
  records = [aws_lb.alb.dns_name]
}



resource "aws_route53_record" "pgadmin" {
  count = var.pgadmin_enabled ? 1 : 0
  zone_id = var.zone_id
  name = "pgadmin.${var.env == "prod" ? "app" : var.env}.${var.domain}"
  type = "CNAME"
  ttl = 60
  records = [aws_lb.alb.dns_name]
}


resource "aws_service_discovery_service" "pgadmin" {
  count = var.pgadmin_enabled ? 1 : 0
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
