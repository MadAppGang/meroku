data "aws_region" "current" {}

resource "aws_scheduler_schedule_group" "group" {
  name = "${var.project}-schedule-group-${var.env}-${var.task}"

  tags = {
    Name        = "${var.project}-schedule-group-${var.env}-${var.task}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_scheduler_schedule" "scheduler" {
  name       = "${var.project}-scheduler-${var.task}-${var.env}"
  group_name = aws_scheduler_schedule_group.group.name

  flexible_time_window {
    mode = "OFF"
  }

  schedule_expression = var.schedule

  target {
    arn      = var.cluster
    role_arn = aws_iam_role.scheduler_role.arn

    ecs_parameters {
      task_definition_arn    = aws_ecs_task_definition.task.arn
      enable_execute_command = true
      launch_type            = "FARGATE"

      network_configuration {
        assign_public_ip = true
        security_groups  = [aws_security_group.task.id]
        subnets          = var.subnet_ids
      }
    }
  }
}

resource "aws_ecr_repository" "task" {
  name  = "${var.project}_task_${var.task}"
  count = var.env == "dev" ? 1 : 0


  tags = {
    Name        = "${var.project}_task_${var.task}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

locals {
  ecr_image    = var.env == "dev" ? join("", aws_ecr_repository.task.*.repository_url) : var.ecr_url
  docker_image = var.docker_image != "" ? var.docker_image : "${local.ecr_image}:latest"
}

resource "aws_ecr_repository_policy" "task" {
  repository = join("", aws_ecr_repository.task.*.name)
  policy     = data.aws_iam_policy_document.default_ecr_policy.json
  count      = var.env == "dev" ? 1 : 0
}

resource "aws_ecs_task_definition" "task" {
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  family                   = var.task
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  container_definitions = jsonencode([merge(
    {
      name      = "${var.project}_container_${var.task}_${var.env}"
      cpu       = 256
      memory    = 512
      image     = local.docker_image
      secrets   = local.task_env_ssm
      essential = true
      Environment = local.environment_variables

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.task.name
          awslogs-stream-prefix = "ecs"
          awslogs-region        = data.aws_region.current.name
        }
      }
    },
    length(var.container_command) > 0 ? { command = var.container_command } : {}
  )])

  tags = {
    Name        = "${var.project}-task-${var.task}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    env         = var.env
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_log_group" "task" {
  name = "${var.project}_task_${var.task}_${var.env}"

  retention_in_days = 7

  tags = {
    Name        = "${var.project}_task_${var.task}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    env         = var.env
    Application = "${var.project}-${var.env}"
  }
}
