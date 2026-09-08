data "aws_region" "current" {}

resource "aws_ecr_repository" "task" {
  name         = "${var.project}_task_${var.task}"
  count        = var.env == "dev" ? 1 : 0
  force_delete = true

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
  # Account+region-global. Distinct "event_task" infix so an event processor
  # and a scheduled task sharing a name do not write revisions into one family.
  family             = "${var.project}_event_task_${var.task}_${var.env}"
  cpu                = 256
  memory             = 512
  execution_role_arn = aws_iam_role.task_execution.arn
  task_role_arn      = aws_iam_role.task.arn

  container_definitions = jsonencode([{
    name        = "${var.project}_container_${var.task}_${var.env}"
    cpu         = 256
    memory      = 512
    image       = local.docker_image
    secrets     = local.task_env_ssm
    essential   = true
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

  # A name may not appear in both `environment` and `secrets` above. ECS refuses
  # the RegisterTaskDefinition call, so without this the apply dies part-way
  # through, naming a variable whoever ran it never touched —
  # env_secret_check.tf has the full account of how a project gets there.
  #
  # It lives on the TASK DEFINITION, matching modules/workloads and for the same
  # reason: this is the resource AWS rejects, so the address Terraform prints
  # alongside the message is the one the reader has to fix. This resource
  # carries no other lifecycle block, which matters — a resource may have only
  # ONE, so anything added here later has to share this block rather than open a
  # second.
  #
  # Meta-argument: no state, no diff. Needs Terraform >= 1.2, which versions.tf
  # requires.
  #
  # Unlike modules/workloads, data.aws_ssm_parameters_by_path.task (env.tf) has
  # no depends_on, so the read is not deferred behind the /env parameter's own
  # creation and the condition is known at plan even on a brand-new environment.
  # There is no first-apply window where it is postponed.
  lifecycle {
    precondition {
      condition     = module.task_env_secret_check.valid["task"]
      error_message = module.task_env_secret_check.message["task"]
    }
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


