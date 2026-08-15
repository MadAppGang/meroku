resource "aws_lb" "alb" {
  count = var.enabled ? 1 : 0

  name               = "${var.project}-alb-${var.env}"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.private_subnets
  idle_timeout       = var.idle_timeout

  tags = {
    Name        = "${var.project}-alb-${var.env}"
    Environment = var.env
    Project     = var.project
    Application = "${var.project}-${var.env}"
    ManagedBy   = "meroku"
  }
}

# Security Group for the ALB.
#
# Deliberately NOT gated on var.enabled, while the load balancer and listener above
# and below are. The backend's security group in modules/workloads carries an ingress
# rule referencing this one, and that reference is what made disabling the ALB fail:
# Terraform does not reliably order an update of the backend group (dropping the rule)
# before the destroy of this group, so it tried the destroy first, AWS refused with
# DependencyViolation while the rule still pointed here, and the apply retried for
# ~14 minutes and then failed. Measured twice: 13m45s and 9m49s.
#
# Keeping the group means disabling the ALB destroys no security group at all, so
# there is no ordering to get wrong. It costs nothing — an empty security group is
# free, and with no load balancer in it, it admits nothing.
#
# A full `terraform destroy` still removes it, and correctly: both groups are
# destroyed together, and destroy-before-destroy IS ordering Terraform gets right.
resource "aws_security_group" "alb" {
  name   = "${var.project}_alb_${var.env}"
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

  tags = {
    Name        = "${var.project}_alb_${var.env}"
    Environment = var.env
    Project     = var.project
    Application = "${var.project}-${var.env}"
    ManagedBy   = "meroku"
  }
}

# Create the HTTP Listener with Redirect to HTTPS
resource "aws_lb_listener" "http" {
  count = var.enabled ? 1 : 0

  load_balancer_arn = aws_lb.alb[0].arn
  port              = "80"
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