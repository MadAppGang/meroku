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

  # Which target group each workload's ALB traffic goes to. `target_type` is
  # ForceNew, and a target group that a listener or a listener rule still
  # references cannot be deleted — AWS answers ResourceInUse and the apply hangs
  # until the timeout. So the "ip" target groups keep target_type = "ip" forever
  # and a bridge-mode workload gets a SEPARATE resource that is "instance" from
  # creation. These two selectors are the only place that choice is made; the
  # four reference sites (the listener default action and the listener rule
  # below, the load_balancer blocks in backend.tf and services.tf) all read them.
  #
  # For every existing environment local.backend_bridge is false and every
  # local.service_bridge entry is false, so both selectors resolve to exactly the
  # resource that is referenced today.
  #
  # try(...) covers enable_alb = false, where every target group has count or
  # for_each zero and a bare index would be an error at plan time rather than a
  # null nobody reads.
  backend_target_group_arn = local.backend_bridge ? try(aws_lb_target_group.backend_instance[0].arn, null) : try(aws_lb_target_group.backend[0].arn, null)

  service_target_group_arns = { for k, _ in local.service_names :
    k => local.service_bridge[k] ? try(aws_lb_target_group.services_instance[k].arn, null) : try(aws_lb_target_group.services[k].arn, null)
  }
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
    target_group_arn = local.backend_target_group_arn
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
    target_group_arn = local.service_target_group_arns[each.key]
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

# Create the Target Group.
#
# target_type stays "ip" here forever — see local.backend_target_group_arn. The
# count gains !local.backend_bridge so this group and its "instance" twin below
# are mutually exclusive; for a Fargate backend local.backend_bridge is false and
# the expression evaluates to today's `var.enable_alb ? 1 : 0`.
resource "aws_lb_target_group" "backend" {
  count = var.enable_alb && !local.backend_bridge ? 1 : 0

  name        = module.naming.names["backend_tg"]
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
    Name        = module.naming.names["backend_tg"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# The bridge-mode twin of the target group above. Identical except for
# target_type and name, and mutually exclusive with it by construction, so no
# live target group ever changes target_type.
#
# `port` is a required argument but is only a placeholder: with
# target_type = "instance" and hostPort = 0 in the task definition, ECS registers
# the instance on the ephemeral port Docker assigned, and that registration
# overrides this. The health check follows it — the health_check block omits
# `port`, which defaults to "traffic-port", so the ALB probes the same dynamic
# port it routes to. ec2_capacity.tf's instance security group already opens
# 32768-65535 from the ALB for exactly this.
resource "aws_lb_target_group" "backend_instance" {
  count = var.enable_alb && local.backend_bridge ? 1 : 0

  # A target-group name is 32 characters at most, and both this group and the
  # "ip" one exist at the same time while a backend is being flipped between
  # runtimes, so the two names must differ.
  name        = module.naming.names["backend_tg_instance"]
  port        = var.backend_image_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "instance"

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
    Name        = module.naming.names["backend_tg_instance"]
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
