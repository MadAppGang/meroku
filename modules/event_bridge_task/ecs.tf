data "aws_region" "current" {}

resource "aws_ecr_repository" "task" {
  name  = "${var.project}_task_${var.task}"
  count = var.env == "dev" ? 1 : 0

  tags = {
    terraform = "true"
  }
}


resource "aws_ecr_repository_policy" "task" {
  repository = join("", aws_ecr_repository.task.*.name)
  policy     = var.ecr_repository_policy
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
    name   = "${var.project}_container_${var.task}_${var.env}"
    cpu    = 256
    memory = 512
    image  = "${var.env == "dev" ? join("", aws_ecr_repository.task.*.repository_url) : var.ecr_url}:latest"
    secrets     = local.task_env_ssm
    essential = true

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
    terraform = "true"
    env       = var.env
  }
}

resource "aws_cloudwatch_log_group" "task" {
  name = "${var.project}_task_${var.task}_${var.env}"

  retention_in_days = 7

  tags = {
    terraform = "true"
    env       = var.env
  }
}


