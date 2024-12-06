locals {
  backend_alb_full_domain_name = "${var.backend_alb_domain_name}.${var.domain}"
}

# Create the HTTPS Listener
resource "aws_lb_listener" "https" {
  count             = var.enable_alb ? 1 : 0

  load_balancer_arn = var.alb_arn
  port              = "443"
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-2016-08"
  certificate_arn   = var.subdomains_certificate_arn

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "Not Found"
      status_code  = "404"
    }
  }
}

# Create the Listener Rule
resource "aws_lb_listener_rule" "https" {
  count             = var.enable_alb ? 1 : 0

  listener_arn = aws_lb_listener.https[0].arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.backend[0].arn
  }

  condition {
    host_header {
      values = [local.backend_alb_full_domain_name]
    }
  }
}

# Create the Target Group
resource "aws_lb_target_group" "backend" {
  count             = var.enable_alb ? 1 : 0

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
  }
}


data "aws_lb" "alb" {
  count             = var.enable_alb ? 1 : 0

  arn               = var.alb_arn 
}

resource "aws_route53_record" "alb_alias" {
  count             = var.enable_alb ? 1 : 0

  zone_id = var.domain_zone_id
  name    = local.backend_alb_full_domain_name
  type    = "A"

  alias {
    name                   = data.aws_lb.alb[0].dns_name
    zone_id                = data.aws_lb.alb[0].zone_id
    evaluate_target_health = true
  }
}