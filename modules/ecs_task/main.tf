data "aws_region" "current" {}

resource "aws_scheduler_schedule_group" "group" {
  name = module.naming.names["schedule_group"]

  tags = {
    Name        = "${var.project}-schedule-group-${var.env}-${var.task}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_scheduler_schedule" "scheduler" {
  name       = module.naming.names["schedule"]
  group_name = aws_scheduler_schedule_group.group.name

  flexible_time_window {
    mode = "OFF"
  }

  schedule_expression          = var.schedule
  schedule_expression_timezone = var.schedule_expression_timezone

  target {
    arn      = var.cluster
    role_arn = aws_iam_role.scheduler_role.arn

    # Opt-in only. Omitted when unset so AWS keeps its own default of 185;
    # emitting a bare default here would cut every existing task's retry budget
    # on the next apply, silently and without anyone asking for it.
    dynamic "retry_policy" {
      for_each = var.max_retry_attempts != null ? [var.max_retry_attempts] : []
      content {
        maximum_retry_attempts = retry_policy.value
      }
    }

    dynamic "dead_letter_config" {
      for_each = var.dlq_arn != "" ? [var.dlq_arn] : []
      content {
        arn = dead_letter_config.value
      }
    }

    ecs_parameters {
      task_definition_arn    = aws_ecs_task_definition.task.arn_without_revision
      enable_execute_command = true
      launch_type            = "FARGATE"

      network_configuration {
        assign_public_ip = var.assign_public_ip
        security_groups  = [aws_security_group.task.id]
        subnets          = var.subnet_ids
      }
    }
  }
}

resource "aws_ecr_repository" "task" {
  name         = "${var.project}_task_${var.task}"
  count        = var.env == "dev" ? 1 : 0
  force_delete = true

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
  # Task definition families are account+region-global. This must match the
  # SCHEDULED_TASK_MAP built in modules/workloads/lambda.tf, which looks up
  # "${var.project}_task_${name}_${var.env}" — with a bare var.task those
  # families never existed, so ECR-push auto-deploy for scheduled tasks
  # could not work.
  family             = "${var.project}_task_${var.task}_${var.env}"
  cpu                = var.cpu
  memory             = var.memory
  execution_role_arn = aws_iam_role.task_execution.arn
  task_role_arn      = aws_iam_role.task.arn

  # `environment`, lower case, matching every other container definition in this
  # repository. This key was spelled `Environment` from 2024 until now and that
  # was NOT a bug: awstypes.ContainerDefinition (aws-sdk-go-v2) carries no JSON
  # struct tags, so the provider's encoding/json decode of this string matches
  # field names case-insensitively and re-serialises the normalised form. The
  # rename is a measured no-op — against provider 5.100.0 both spellings plan to
  # the identical state string, `"environment":[{"name":"AWS_REGION",...}]`, and
  # planning either one against a state holding it reports "No changes". No
  # ForceNew, no new revision, nothing to migrate.
  #
  # It is renamed anyway because it is not self-evidently harmless: `Enviroment`,
  # a real typo, silently yields no `environment` key at all and no error, so an
  # unfamiliar reader cannot tell the two cases apart by looking. Disproving this
  # one cost a full investigation with a captured HTTP request. Nobody should
  # have to repeat it.
  container_definitions = jsonencode([merge(
    {
      name        = "${var.project}_container_${var.task}_${var.env}"
      cpu         = var.cpu
      memory      = var.memory
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

  # A name may not appear in both `environment` and `secrets` above. ECS refuses
  # the RegisterTaskDefinition call, so without this the apply dies part-way
  # through, naming a variable whoever ran it never touched —
  # env_secret_check.tf has the full account of how a project gets there.
  #
  # It lives on the TASK DEFINITION, matching modules/workloads and
  # modules/event_bridge_task and for the same reason: this is the resource AWS
  # rejects, so the address Terraform prints alongside the message is the one the
  # reader has to fix. This resource carries no other lifecycle block, which
  # matters — a resource may have only ONE, so anything added here later has to
  # share this block rather than open a second.
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
    Name        = "${var.project}_task_${var.task}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    env         = var.env
    Application = "${var.project}-${var.env}"
  }
}
