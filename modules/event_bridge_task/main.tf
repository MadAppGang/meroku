# Merge legacy single-rule format with new multi-rule format
# This allows backward compatibility while supporting the new rules[] array
locals {
  # Convert legacy format to a rule if rule_name is provided
  legacy_rule = var.rule_name != "" ? {
    "${var.rule_name}" = {
      sources      = var.sources
      detail_types = var.detail_types
    }
  } : {}

  # Merge legacy rule with new rules - new rules take precedence
  all_rules = merge(local.legacy_rule, var.rules)
}

resource "aws_cloudwatch_event_rule" "rule" {
  for_each = local.all_rules

  name = module.naming.names["rule_${each.key}"]

  event_pattern = jsonencode({
    source      = each.value.sources
    detail-type = each.value.detail_types
  })

  tags = {
    Name        = "${var.project}-rule-${each.key}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_event_target" "target" {
  for_each = local.all_rules

  arn      = var.cluster
  rule     = aws_cloudwatch_event_rule.rule[each.key].name
  role_arn = aws_iam_role.task_execution.arn

  ecs_target {
    task_count          = var.task_count
    task_definition_arn = aws_ecs_task_definition.task.arn

    network_configuration {
      assign_public_ip = var.assign_public_ip
      security_groups  = [aws_security_group.task.id]
      subnets          = var.subnet_ids
    }

    launch_type = "FARGATE"
  }
}

resource "aws_security_group" "task" {
  name   = "${var.project}_${var.task}_${var.env}"
  vpc_id = var.vpc_id

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  tags = {
    Name        = "${var.project}-${var.task}-sg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


