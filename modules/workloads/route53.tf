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
  name =  "mockoon" 
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
  name = "api.${var.env}.${var.domain}"
  type = "CNAME"
  ttl = 60
  records = [aws_lb.alb.dns_name]
}
