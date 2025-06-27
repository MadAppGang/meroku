data "aws_region" "current" {}

resource "aws_ecr_repository" "task" {
  name  = "${var.project}_task_${var.task}"
  count = var.env == "dev" ? 1 : 0

  tags = {
    Name        = "${var.project}-task-${var.task}-ecr"
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

  container_definitions = jsonencode([{
    name      = "${var.project}_container_${var.task}_${var.env}"
    cpu       = 256
    memory    = 512
    image     = local.docker_image
    secrets   = local.task_env_ssm
    essential = true
    environment = local.environment_variables

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.task.name
        awslogs-stream-prefix = "ecs"
        awslogs-region        = data.aws_region.current.name
      }
    }

  }])

  tags = {
    Name        = "${var.project}-task-${var.task}-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_log_group" "task" {
  name = "${var.project}_task_${var.task}_${var.env}"

  retention_in_days = 7

  tags = {
    Name        = "${var.project}-task-${var.task}-logs-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}


