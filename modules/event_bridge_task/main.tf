resource "aws_cloudwatch_event_rule" "rule" {
  name = "${var.project}_rule_${var.rule_name}_${var.env}"

  event_pattern = jsonencode({
    source      = var.sources
    detail-type = var.detail_types
  })
}

resource "aws_cloudwatch_event_target" "target" {
  arn      = var.cluster
  rule     = aws_cloudwatch_event_rule.rule.name
  role_arn = aws_iam_role.task_execution.arn

  ecs_target {
    task_count          = var.task_count
    task_definition_arn = aws_ecs_task_definition.task.arn

    network_configuration {
      assign_public_ip = var.allow_public_access
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
}


