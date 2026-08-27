locals {
  service_names = { for service in var.services : service.name => service }

  # Target-group names come from modules/naming; see naming.tf for the request
  # map that defines them. Only the lookup lives here.
  service_tg_name          = { for k in keys(local.service_names) : k => module.naming.names["service_tg_${k}"] }
  service_tg_instance_name = { for k in keys(local.service_names) : k => module.naming.names["service_tg_instance_${k}"] }

  # Every target group that will actually exist: the two maps above are
  # complementary, so this is exactly one name per service.
  #
  # The cascade lets forms MIX — a service short enough keeps its historical
  # name while a longer one drops the decoration — and a service name that
  # spells out another service's decorated form ("service-<x>-tg", "svc-<x>-i")
  # can then land on a name already taken. It takes a perverse name to hit, but
  # without this check the failure is a DuplicateTargetGroupName from the AWS
  # API partway through an apply, naming neither service.
  active_tg_names = [
    for k, v in local.service_names :
    local.service_bridge[k] ? local.service_tg_instance_name[k] : local.service_tg_name[k]
  ]

  duplicate_tg_names = [
    for n in distinct(local.active_tg_names) : n
    if length([for m in local.active_tg_names : m if m == n]) > 1
  ]

  tg_name_collision_message = <<-EOT
    Two services resolve to the same ALB target-group name: ${join(", ", local.duplicate_tg_names)}.

    Target-group names are capped at 32 characters, so a service whose own name
    is long is rendered in a shortened form, and a service name that spells out
    another service's decorated form ("service-<x>-tg", "svc-<x>-i") can land on
    a name already taken.

    Rename one of the colliding services in the `services:` block of your
    environment YAML — anything not of the form "service-<x>-tg" or "svc-<x>-i"
    will do — then re-run `task infra-gen-<env>`. AWS would otherwise reject
    this with DuplicateTargetGroupName partway through the apply.
  EOT
}

# Create ALB target group for each service.
#
# target_type stays "ip" here forever — it is ForceNew, and a target group a
# listener rule still references cannot be deleted. A bridge-mode service gets
# the separate "instance" resource below instead; the two for_each expressions
# are complementary, so exactly one exists per service. For every Fargate service
# local.service_bridge[k] is false and this renders today's set.
resource "aws_lb_target_group" "services" {
  for_each = { for k, v in local.service_names : k => v if var.enable_alb && !local.service_bridge[k] }

  name        = local.service_tg_name[each.key]
  port        = each.value.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  # Repeated on the instance twin below rather than hoisted into a `check`
  # block: `check` only warns, and this must stop the apply. Between the two
  # resources every service with a target group is covered.
  lifecycle {
    precondition {
      condition     = length(local.duplicate_tg_names) == 0
      error_message = local.tg_name_collision_message
    }
  }

  health_check {
    enabled             = true
    healthy_threshold   = 2
    interval            = 30
    matcher             = "200"
    path                = "/health/live"
    port                = "traffic-port"
    protocol            = "HTTP"
    timeout             = 5
    unhealthy_threshold = 10
  }

  tags = {
    Name        = local.service_tg_name[each.key]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# The bridge-mode twin. Identical except for target_type and name.
#
# `port` is required but is a placeholder: with hostPort = 0 in the task
# definition, ECS registers the instance on the ephemeral port Docker assigned
# and that overrides it. health_check already probes "traffic-port", so it
# follows the registration with no further change, and ec2_capacity.tf's instance
# security group opens 32768-65535 from the ALB for exactly this.
resource "aws_lb_target_group" "services_instance" {
  for_each = { for k, v in local.service_names : k => v if var.enable_alb && local.service_bridge[k] }

  # 32 characters is the AWS limit for a target-group name, and both groups exist
  # at once while a service is being flipped between runtimes, so the names must
  # differ: "-svc-<name>-i-" rather than "-service-<name>-tg-". Shortening for a
  # name that overflows even that form is modules/naming's job; the precondition
  # below is what proves the two stayed apart.
  name        = local.service_tg_instance_name[each.key]
  port        = each.value.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "instance"

  lifecycle {
    precondition {
      condition     = length(local.duplicate_tg_names) == 0
      error_message = local.tg_name_collision_message
    }
  }

  health_check {
    enabled             = true
    healthy_threshold   = 2
    interval            = 30
    matcher             = "200"
    path                = "/health/live"
    port                = "traffic-port"
    protocol            = "HTTP"
    timeout             = 5
    unhealthy_threshold = 10
  }

  tags = {
    Name        = local.service_tg_instance_name[each.key]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# NOTE: ECR repository creation moved to ecr.tf for per-service configuration (Schema v10)
# ECR repositories are now managed in ecr.tf with support for:
# - create_ecr: Create dedicated ECR repository
# - manual_repo: Use existing ECR repository URI
# - use_existing: Share ECR from another service/task

# Service Discovery for each service
resource "aws_service_discovery_service" "services" {
  for_each = local.service_names

  name = "${var.project}_service_${each.key}_${var.env}"

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.local.id

    # See aws_service_discovery_service.backend in backend.tf: an A record cannot
    # describe a bridge task, whose host port is chosen at placement time, so the
    # bridge branch is SRV-only. One Cloud Map service either way, so the name
    # SERVICE_INTERNAL_URL and the API Gateway private integration resolve does
    # not change, and service_registries is never removed from a live service.
    #
    # ORDER IS LOAD-BEARING: dns_records is a LIST compared positionally. A before
    # SRV. Swapping them replaces every Cloud Map service in the environment.
    dynamic "dns_records" {
      for_each = local.service_bridge[each.key] ? [] : [1]
      content {
        ttl  = 10
        type = "A"
      }
    }

    # Always, in both network modes.
    dns_records {
      ttl  = 10
      type = "SRV"
    }

    routing_policy = "MULTIVALUE"
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

# Create ECS Service for each service
resource "aws_ecs_service" "services" {
  for_each = local.service_names

  name                               = "${var.project}_service_${each.key}_${var.env}"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.services[each.key].arn
  desired_count                      = each.value.desired_count
  deployment_minimum_healthy_percent = 50
  scheduling_strategy                = "REPLICA"
  enable_ecs_managed_tags            = true
  enable_execute_command             = each.value.remote_access

  # See aws_ecs_service.backend in backend.tf for why these three are shaped the
  # way they are. In short: launch_type and capacity_provider_strategy are
  # strictly either/or with no ConflictsWith to enforce it, the strategy appears
  # only on the EC2 branch because under the pinned provider a service gaining
  # its first strategy is ForceNew, and force_new_deployment is EC2-only because
  # writing true into an existing Fargate service is a diff in every environment
  # that has ever applied.
  launch_type          = local.service_pools[each.key] == null ? "FARGATE" : null
  force_new_deployment = local.service_pools[each.key] != null

  dynamic "capacity_provider_strategy" {
    for_each = local.service_pools[each.key] == null ? [] : [local.service_pools[each.key]]
    content {
      capacity_provider = aws_ecs_capacity_provider.pool[capacity_provider_strategy.value].name

      # An omitted weight defaults to 0 via the API, and a strategy where every
      # weight is 0 fails every CreateService and RunTask.
      weight = 1
      base   = 0
    }
  }

  # awsvpc only — AWS rejects networkConfiguration on a task whose network mode
  # is not awsvpc. assign_public_ip is an expression rather than an omission
  # because a dynamic block has one content and an argument in it cannot be
  # conditionally absent: true for Fargate exactly as today, false (-> DISABLED)
  # for an awsvpc EC2 pool, which the API accepts — only ENABLED is rejected.
  dynamic "network_configuration" {
    for_each = local.service_network_modes[each.key] == "awsvpc" ? [1] : []
    content {
      security_groups = [aws_security_group.services[each.key].id]
      subnets         = var.subnet_ids
      # Fargate AND the egress strategy must both want an address. See the same
      # expression in backend.tf, and ai_docs/EGRESS_COST_MODEL.md.
      assign_public_ip = local.service_pools[each.key] == null && var.assign_public_ip
    }
  }

  dynamic "load_balancer" {
    for_each = var.enable_alb ? [1] : []
    content {
      target_group_arn = local.service_target_group_arns[each.key]
      container_name   = "${var.project}_service_${each.key}_${var.env}"
      container_port   = each.value.container_port
    }
  }

  # EC2 only, so the Fargate plan is unchanged.
  dynamic "ordered_placement_strategy" {
    for_each = local.service_pools[each.key] == null ? [] : [1]
    content {
      type  = "binpack"
      field = "memory"
    }
  }

  service_registries {
    registry_arn   = aws_service_discovery_service.services[each.key].arn
    container_name = "${var.project}_service_${each.key}_${var.env}"
    container_port = each.value.container_port
  }

  tags = {
    Name        = "${var.project}-service-${each.key}-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }

  # See aws_ecs_service.backend in backend.tf: CI owns the running revision,
  # Terraform owns the service around it. Without this an apply silently rolls
  # back whatever the CI Lambda last deployed.
  #
  # The precondition shares the block because a resource may carry only one
  # lifecycle block. Every other pool read is total, so this is the single site
  # that can fail on a compute_pool naming a pool that is missing or disabled.
  lifecycle {
    ignore_changes = [task_definition]

    precondition {
      condition     = local.service_pools[each.key] == null || contains(keys(local.pools), local.service_pools[each.key])
      error_message = "Service \"${each.key}\" sets runtime \"ec2\" with compute pool \"${local.service_pool_names[each.key]}\", which is not an enabled compute pool. Add a pool with that name under compute.pools, set enabled: true on it if it is disabled, or point the service at one of these: ${join(", ", keys(local.pools))}."
    }
  }

  # The service must never be created before the cluster knows the capacity
  # provider its strategy names. With no pools this resource has count = 0 and
  # the dependency contributes no edge.
  depends_on = [aws_ecs_cluster_capacity_providers.main]
}

# Create Task Definition for each service
resource "aws_ecs_task_definition" "services" {
  for_each = local.service_names

  # "awsvpc" for Fargate and for a pool that overrode its network mode, "bridge"
  # otherwise. Every existing environment renders the identical string.
  #
  # No cpu/memory floors here, unlike backend.tf: these already pass through
  # untouched, because the 256/512 minimums are a Fargate task-size rule that was
  # only ever applied to the backend.
  network_mode             = local.service_network_modes[each.key]
  requires_compatibilities = local.service_pools[each.key] == null ? ["FARGATE"] : ["EC2"]
  family                   = "${var.project}_service_${each.key}_${var.env}"
  cpu                      = each.value.cpu
  memory                   = each.value.memory
  execution_role_arn       = aws_iam_role.services_task_execution[each.key].arn
  task_role_arn            = aws_iam_role.services_task[each.key].arn

  # local.service_arm64 is the total per-service local from ec2_capacity.tf. This
  # site is outside the reach of the ECS service's precondition, so a bad pool
  # name must render false rather than raise `Error: Invalid index`.
  dynamic "runtime_platform" {
    for_each = local.service_arm64[each.key] ? [1] : []
    content {
      cpu_architecture        = "ARM64"
      operating_system_family = "LINUX"
    }
  }

  container_definitions = jsonencode(concat(
    each.value.xray_enabled ? local.xray_service_container : [],
    [{
      name   = "${var.project}_service_${each.key}_${var.env}"
      cpu    = each.value.cpu
      memory = each.value.memory
      # Use resolved ECR URL from ecr.tf (supports create_ecr, manual_repo, use_existing modes)
      #
      # `:latest` belongs INSIDE the ECR branch, and the parentheses are load
      # bearing. A user-supplied docker_image usually carries its own tag, so
      # appending here would render "public.ecr.aws/nginx/nginx:stable:latest" and
      # kill the task with CannotPullImageManifestError: invalid reference format.
      # Only the URL meroku resolved itself is untagged. Same shape as
      # modules/ecs_task/main.tf and modules/event_bridge_task/ecs.tf.
      image   = each.value.docker_image != "" ? each.value.docker_image : "${local.service_ecr_urls[each.key]}:latest"
      command = each.value.container_command


      // we support three types of env variables:
      // 1. from SSM
      // 2. from env_files_s3
      // 3. from env_vars variable
      secrets = local.services_env_ssm[each.key]
      environment = concat(local.services_env, [
        for name, value in each.value.env_vars : {
          name  = name
          value = value
        }
        ], [
        { name = "EVENT_SOURCE", value = local.services_event_source[each.key] },
        { name = "SERVICE_INTERNAL_URL", value = local.services_internal_domain[each.key] },
        { name = "SERVICE_NAME", value = each.key }
      ])
      environmentFiles = [
        for file in local.services_env_files_s3[each.key] : {
          value = "arn:aws:s3:::${file.bucket}/${file.key}"
          type  = "s3"
        }
      ]
      essential = each.value.essential

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.services[each.key].name
          awslogs-stream-prefix = "ecs"
          awslogs-region        = data.aws_region.current.name
        }
      }

      # host_port keeps its meaning on the awsvpc branch and is IGNORED on the
      # bridge branch, where the host port is chosen at placement time. A fixed
      # hostPort under bridge binds that port on the instance, so a second task
      # of the same service cannot place there and the ALB cannot tell two tasks
      # on one host apart; hostPort = 0 asks Docker for an ephemeral port, which
      # is what target_type = "instance" registration and Cloud Map SRV read.
      # The validator does not refuse a set host_port on a bridge service — a
      # service can move between pools, and refusing a now-meaningless value
      # would block the move.
      portMappings = [{
        protocol      = "tcp"
        containerPort = each.value.container_port
        hostPort      = local.service_bridge[each.key] ? 0 : each.value.host_port
        name          = "${var.project}_service_${each.key}_${var.env}"
      }]
    }]
  ))

  tags = {
    Name        = "${var.project}-service-${each.key}-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# Create Security Group for each service
resource "aws_security_group" "services" {
  for_each = local.service_names

  name   = "${var.project}_service_${each.key}_${var.env}"
  vpc_id = var.vpc_id

  ingress {
    protocol         = "tcp"
    from_port        = each.value.container_port
    to_port          = each.value.container_port
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
    Name        = "${var.project}-service-${each.key}-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# Create CloudWatch Log Group for each service
resource "aws_cloudwatch_log_group" "services" {
  for_each = local.service_names

  name              = "${var.project}_service_${each.key}_${var.env}"
  retention_in_days = 7

  tags = {
    Name        = "${var.project}-service-${each.key}-tg-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

# Create IAM roles for each service
resource "aws_iam_role" "services_task" {
  for_each = local.service_names

  name               = module.naming.names["service_task_role_${each.key}"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}_service_${each.key}_task_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role" "services_task_execution" {
  for_each = local.service_names

  name               = module.naming.names["service_task_execution_role_${each.key}"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}_service_${each.key}_task_execution_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Attach policies to service roles
resource "aws_iam_role_policy_attachment" "services_task_execution" {
  for_each = local.service_names

  role       = aws_iam_role.services_task_execution[each.key].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "services_task_cloudwatch" {
  for_each = local.service_names

  role       = aws_iam_role.services_task[each.key].name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

# S3 bucket access
resource "aws_iam_role_policy_attachment" "service_task_bucket" {
  for_each = local.service_names

  role       = aws_iam_role.services_task_execution[each.key].name
  policy_arn = aws_iam_policy.full_access_to_backend_bucket.arn
}

# SES access for all services
resource "aws_iam_role_policy_attachment" "service_task_ses" {
  for_each = local.service_names

  role       = aws_iam_role.services_task_execution[each.key].name
  policy_arn = aws_iam_policy.send_emails.arn
}



# SSM parameter access for services
resource "aws_iam_role_policy_attachment" "services_ssm_parameter_access" {
  for_each = local.service_names

  role       = aws_iam_role.services_task_execution[each.key].name
  policy_arn = aws_iam_policy.services_ssm_parameter_access[each.key].arn
}

resource "aws_iam_policy" "services_ssm_parameter_access" {
  for_each = local.service_names

  name = "ServiceSSMAccessPolicy_${var.project}_${each.key}_${var.env}"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
        Resource = ["arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/${each.key}/*"]
      }
    ]
  })

  tags = {
    Name        = "ServiceSSMAccessPolicy_${var.project}_${each.key}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "services_sqs_access" {
  for_each = { for k, v in local.service_names : k => v if var.sqs_enable }

  role       = aws_iam_role.services_task[each.key].name
  policy_arn = var.sqs_policy_arn
}


# S3 env files access for services
resource "aws_iam_role_policy" "services_s3_env" {
  for_each = { for k, v in local.service_names : k => v if length(local.env_files_s3) > 0 }

  name = "${var.project}_${each.key}_s3_env_${var.env}"
  role = aws_iam_role.services_task_execution[each.key].name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:*"
        ]
        Resource = [
          for file in local.env_files_s3 :
          "arn:aws:s3:::${file.bucket}/${file.key}"
        ]
      }
    ]
  })
}

// create empty files if they don't exist for each env files services
resource "null_resource" "create_services_env_files" {
  for_each = {
    for pair in flatten([
      for service_name, files in local.services_env_files_s3 : [
        for file in files : {
          id     = "${file.bucket}-${file.key}"
          bucket = file.bucket
          key    = file.key
        }
      ]
    ]) : pair.id => pair
  }

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      echo "Checking if file exists: ${each.value.bucket}/${each.value.key}"
      touch empty.tmp
      if ! aws s3api head-object --bucket ${each.value.bucket} --key ${each.value.key} 2>/dev/null; then
        aws s3api put-object --bucket ${each.value.bucket} --key ${each.value.key} --body empty.tmp
      fi
      rm -f empty.tmp
    EOT
  }

  triggers = {
    bucket = each.value.bucket
    key    = each.value.key
  }
}

# Remote exec policy for services
resource "aws_iam_role_policy" "services_ecs_exec_policy" {
  for_each = { for k, v in local.service_names : k => v if v.remote_access }

  name = "${var.project}-${each.key}-ecs-exec-policy-${var.env}"
  role = aws_iam_role.services_task[each.key].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "ssmmessages:CreateControlChannel",
          "ssmmessages:CreateDataChannel",
          "ssmmessages:OpenControlChannel",
          "ssmmessages:OpenDataChannel"
        ]
        Resource = "*"
      }
    ]
  })
}

# EventBridge permissions to allow services to emit and listen to events
resource "aws_iam_policy" "services_eventbridge" {
  for_each = local.service_names

  name = "EventBridgeAccess_${var.project}_${each.key}_${var.env}"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "events:PutEvents",
          "events:DescribeEventBus",
          "events:ListRules",
          "events:DescribeRule",
          "events:ListTargetsByRule",
          "events:ListEventBuses"
        ]
        Resource = [
          "arn:aws:events:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:event-bus/default",
          "arn:aws:events:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:event-bus/${var.project}-*",
          "arn:aws:events:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:rule/*"
        ]
      }
    ]
  })

  tags = {
    Name        = "EventBridgeAccess_${var.project}_${each.key}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "services_task_eventbridge" {
  for_each = local.service_names

  role       = aws_iam_role.services_task[each.key].name
  policy_arn = aws_iam_policy.services_eventbridge[each.key].arn
}
