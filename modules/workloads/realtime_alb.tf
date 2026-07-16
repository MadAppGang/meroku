# Realtime (SSE / streaming) ALB — Option A.
#
# Additive, gated on var.enable_realtime_alb. When enabled, a dedicated public
# Application Load Balancer is created in front of the backend ECS task for
# long-lived SSE / streaming connections, with a large idle timeout. The
# existing API Gateway + Cloud Map path is left fully intact — the backend task
# is registered with BOTH this target group AND Cloud Map (see backend.tf).
#
# The realtime hostname is <realtime_subdomain_prefix>.<domain-module zone>
# (e.g. realtime.dev.example.com on env-prefixed setups, realtime.example.com
# on prod where add_env_domain_prefix = false), covered by the wildcard
# certificate *.<zone> from the domain module (var.subdomains_certificate_arn).

locals {
  # Parent domain for the realtime hostname. Preferred source: the domain
  # module's actual zone name via var.realtime_parent_domain (env-prefixed only
  # when the domain module prefixes — prod sets add_env_domain_prefix = false).
  # Fallback keeps the legacy "<env>.<domain>" derivation for callers that do
  # not pass the variable.
  realtime_parent_domain = coalesce(var.realtime_parent_domain, "${var.env}.${var.domain}")
  realtime_full_domain   = "${var.realtime_subdomain_prefix}.${local.realtime_parent_domain}"
}

resource "aws_lb" "realtime" {
  count = var.enable_realtime_alb ? 1 : 0

  name               = "${var.project}-realtime-${var.env}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.realtime_alb[0].id]
  subnets            = var.subnet_ids
  idle_timeout       = var.realtime_alb_idle_timeout

  tags = {
    Name        = "${var.project}-realtime-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_security_group" "realtime_alb" {
  count = var.enable_realtime_alb ? 1 : 0

  name   = "${var.project}_realtime_alb_${var.env}"
  vpc_id = var.vpc_id

  ingress {
    protocol         = "tcp"
    from_port        = 443
    to_port          = 443
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
    description      = "Allow HTTPS from anywhere"
  }

  ingress {
    protocol         = "tcp"
    from_port        = 80
    to_port          = 80
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
    description      = "Allow HTTP from anywhere (redirected to HTTPS)"
  }

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name        = "${var.project}_realtime_alb_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_lb_target_group" "realtime" {
  count = var.enable_realtime_alb ? 1 : 0

  name                 = "${var.project}-realtime-tg-${var.env}"
  port                 = var.backend_image_port
  protocol             = "HTTP"
  protocol_version     = "HTTP1"
  vpc_id               = var.vpc_id
  target_type          = "ip"
  deregistration_delay = 30

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

  # No stickiness: SSE connections are pinned for their lifetime to whichever
  # task accepted them; round-robin spreads new connections across tasks.

  tags = {
    Name        = "${var.project}-realtime-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_lb_listener" "realtime_https" {
  count = var.enable_realtime_alb ? 1 : 0

  load_balancer_arn = aws_lb.realtime[0].arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.subdomains_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.realtime[0].arn
  }
}

resource "aws_lb_listener" "realtime_http" {
  count = var.enable_realtime_alb ? 1 : 0

  load_balancer_arn = aws_lb.realtime[0].arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_route53_record" "realtime_alias" {
  count = var.enable_realtime_alb ? 1 : 0

  zone_id = var.domain_zone_id
  name    = local.realtime_full_domain
  type    = "A"

  alias {
    name                   = aws_lb.realtime[0].dns_name
    zone_id                = aws_lb.realtime[0].zone_id
    evaluate_target_health = true
  }
}
