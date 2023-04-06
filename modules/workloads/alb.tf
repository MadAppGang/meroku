data "aws_route53_zone" "domain" {
  name  = var.domain
}

resource "aws_lb" "alb" {
  name               = "${var.project}-alb-${var.env}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.subnet_ids

  enable_deletion_protection = false
}

data "aws_acm_certificate" "amazon_issued_domain" {
  domain      = "*.${var.env == "prod" ? "" : format("%s.", var.env)}${var.domain}"
  types       = ["AMAZON_ISSUED"]
  most_recent = true
}


resource "aws_alb_listener" "http" {
  load_balancer_arn = aws_lb.alb.id
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"

    redirect {
      port        = 443
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_alb_listener" "https" {
  load_balancer_arn = aws_lb.alb.arn
  port              = 443
  protocol          = "HTTPS"

  ssl_policy      = "ELBSecurityPolicy-2016-08"
  certificate_arn = data.aws_acm_certificate.amazon_issued_domain.arn

  default_action {
    target_group_arn = aws_alb_target_group.backend.id
    type             = "forward"
  }
}

resource "aws_lb_listener_rule" "api" {
  listener_arn = aws_alb_listener.https.arn
  priority     = 100

  action {
    type             = "forward"
    target_group_arn = aws_alb_target_group.backend.arn
  }

  condition {
    host_header {
      values = ["api.${var.env == "prod" ? "" : format("%s.", var.env)}${var.domain}"]
    }
  }
}


resource "aws_lb_listener_rule" "mockoon" {
  listener_arn = aws_alb_listener.https.arn
  priority     = 110

  action {
    type             = "forward"
    target_group_arn = aws_alb_target_group.mockoon.arn
  }

  condition {
    path_pattern {
      values = ["/mock/*"]
    }
  }

}


resource "aws_security_group" "alb" {
  name   = "${var.project}_sg_alb_${var.env}"
  vpc_id = var.vpc_id

  ingress {
    protocol         = "tcp"
    from_port        = 80
    to_port          = 80
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  ingress {
    protocol         = "tcp"
    from_port        = 443
    to_port          = 443
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}
