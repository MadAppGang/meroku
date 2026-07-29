locals {
  # The ALB and API Gateway are mutually exclusive ingress paths for the backend, so the
  # ALB serves the SAME hostname API Gateway would have served (var.api_domain). Flipping
  # enable_alb therefore does not change the public API URL — no client has to be repointed.
  alb_primary_domain = var.api_domain

  # Optional extra hostname (backend_alb_domain_name), qualified with the env-resolved
  # domain from the domain module. Never re-derive the "<env>." prefix here: that rule
  # lives in modules/domain (add_env_domain_prefix) and must stay in one place, or the
  # record lands in the wrong zone and the wildcard cert stops covering it.
  alb_extra_domain = var.backend_alb_domain_name != "" && var.env_domain != "" ? "${var.backend_alb_domain_name}.${var.env_domain}" : ""
}

# HTTPS listener. The wildcard cert (*.<env_domain>) covers both the api_domain and any
# extra host, so a single certificate serves every rule below.
resource "aws_lb_listener" "https" {
  count = var.enable_alb ? 1 : 0

  load_balancer_arn = var.alb_arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-2016-08"
  certificate_arn   = var.subdomains_certificate_arn

  # Catch-all to the backend, mirroring API Gateway's `ANY /{proxy+}` backend route.
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.backend[0].arn
  }
}

# One rule per named service, mirroring API Gateway's `ANY /<name>/{proxy+}` routes so the
# URL layout is identical on both ingress paths. Priority is derived from the sorted
# service names, keeping it stable across plans as services are added or removed.
resource "aws_lb_listener_rule" "services" {
  for_each = { for k, v in local.service_names : k => v if var.enable_alb }

  listener_arn = aws_lb_listener.https[0].arn
  priority     = index(sort(keys(local.service_names)), each.key) + 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.services[each.key].arn
  }

  condition {
    path_pattern {
      values = ["/${each.key}/*"]
    }
  }

  tags = {
    Name        = "${var.project}-service-${each.key}-rule-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Create the Target Group
resource "aws_lb_target_group" "backend" {
  count = var.enable_alb ? 1 : 0

  name        = "${var.project}-backend-tg-${var.env}"
  port        = var.backend_image_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  dynamic "health_check" {
    for_each = var.backend_health_endpoint != "" ? [1] : []
    content {
      path                = var.backend_health_endpoint
      interval            = 30
      timeout             = 5
      healthy_threshold   = 2
      unhealthy_threshold = 2
      matcher             = "200"
    }
  }
  stickiness {
    type            = "lb_cookie"
    cookie_duration = 86400
    enabled         = true
  }

  tags = {
    Name        = "${var.project}-backend-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


data "aws_lb" "alb" {
  count = var.enable_alb ? 1 : 0

  arn = var.alb_arn
}

# The API hostname points at the ALB instead of API Gateway. domain.tf drops its own
# record for the same name when enable_alb is set, so exactly one of them exists.
resource "aws_route53_record" "alb_alias" {
  count = var.enable_alb && local.alb_primary_domain != "" ? 1 : 0

  zone_id = var.domain_zone_id
  name    = local.alb_primary_domain
  type    = "A"

  alias {
    name                   = data.aws_lb.alb[0].dns_name
    zone_id                = data.aws_lb.alb[0].zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "alb_extra_alias" {
  count = var.enable_alb && local.alb_extra_domain != "" ? 1 : 0

  zone_id = var.domain_zone_id
  name    = local.alb_extra_domain
  type    = "A"

  alias {
    name                   = data.aws_lb.alb[0].dns_name
    zone_id                = data.aws_lb.alb[0].zone_id
    evaluate_target_health = true
  }
}
