locals {
  backend_name = "${var.project}_service_${var.env}"
}

data "aws_vpc" "selected" {
  id = var.vpc_id
}


resource "aws_ecs_service" "backend" {
  name                               = local.backend_name
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.backend.arn
  desired_count                      = var.backend_desired_count
  deployment_minimum_healthy_percent = 50
  scheduling_strategy                = "REPLICA"
  enable_ecs_managed_tags            = true
  enable_execute_command             = var.backend_remote_access

  # launch_type and capacity_provider_strategy are STRICTLY either/or, and the
  # conditional below is what enforces it: there is no ConflictsWith on either
  # attribute, so emitting both passes `terraform validate` and then fails or
  # misbehaves at apply. launch_type is ForceNew + Computed, which is why the
  # Fargate branch keeps the literal string it has today rather than becoming a
  # dynamic block of its own.
  #
  # Emitting the strategy ONLY on the EC2 branch is the whole zero-diff
  # property. Under the pinned provider (>= 5.34.0, < 6.0.0 — see versions.tf)
  # capacityProviderStrategyCustomizeDiff takes the `ol == 0 && nl > 0` branch —
  # a service with no strategy gaining one — and sets ForceNew. Emitting it
  # universally would therefore turn every already-deployed Fargate service in
  # every environment into a forced replacement on the first apply after
  # upgrade.
  launch_type = local.backend_pool == null ? "FARGATE" : null

  # EC2 branch ONLY, and that is deliberate rather than an oversight.
  #
  # What it prevents: the same diff function returns at its FIRST branch when
  # force_new_deployment is false, so with the flag off, ANY change to
  # capacity_provider_strategy — renaming a pool, editing a weight or a base —
  # replaces the service, and the destroy of the old capacity provider then
  # deadlocks against the still-referencing service. With it on, a
  # strategy -> strategy edit is `ol > 0 && nl > 0`, the in-place branch, so
  # those edits become UpdateService calls with a rolling deployment.
  #
  # Why not unconditionally true: the attribute is Optional with Default false
  # in the provider schema, so it is stored in state. Writing true into every
  # existing Fargate service is an in-place change on first plan
  # (~ force_new_deployment = false -> true) in every environment that has ever
  # applied, plus one unrequested rolling redeployment per service. Gating it on
  # the EC2 branch keeps the Fargate plan byte-identical and gives a new EC2
  # service the flag from creation, so there is no transition to diff.
  force_new_deployment = local.backend_pool != null

  dynamic "capacity_provider_strategy" {
    for_each = local.backend_pool == null ? [] : [local.backend_pool]
    content {
      capacity_provider = aws_ecs_capacity_provider.pool[capacity_provider_strategy.value].name

      # weight is NOT optional decoration. Via the API an omitted weight
      # defaults to 0, and a strategy in which every weight is 0 makes every
      # CreateService and RunTask fail.
      weight = 1
      base   = 0
    }
  }

  # awsvpc only. AWS rejects networkConfiguration on a task whose network mode is
  # not awsvpc, so under bridge the block must not render at all — which is why
  # this is a dynamic block rather than a conditional argument.
  #
  # assign_public_ip is an EXPRESSION rather than an omission because HCL cannot
  # conditionally drop an argument from a dynamic block's single `content`. It is
  # true for Fargate, exactly as today, and false (-> DISABLED) for an awsvpc EC2
  # pool, which the ECS API accepts; only ENABLED is rejected for EC2.
  dynamic "network_configuration" {
    for_each = local.backend_network_mode == "awsvpc" ? [1] : []
    content {
      security_groups = [aws_security_group.backend.id]
      subnets         = var.subnet_ids
      # Two independent conditions, and both have to hold:
      #   local.backend_pool == null -> Fargate. ECS rejects ENABLED for EC2.
      #   var.assign_public_ip       -> the egress strategy wants an address.
      # A NAT strategy sets the second false, which is what takes the task off
      # its own public address. See ai_docs/EGRESS_COST_MODEL.md.
      assign_public_ip = local.backend_pool == null && var.assign_public_ip
    }
  }

  dynamic "load_balancer" {
    for_each = var.enable_alb ? [1] : []
    content {
      target_group_arn = local.backend_target_group_arn
      container_name   = local.backend_name
      container_port   = var.backend_image_port
    }
  }

  # EC2 only, so the Fargate plan is unchanged. AWS: "the binpack strategy is the
  # most efficient strategy in terms of capacity" — which is the entire point of
  # paying for instances by the hour instead of by the task.
  dynamic "ordered_placement_strategy" {
    for_each = local.backend_pool == null ? [] : [1]
    content {
      type  = "binpack"
      field = "memory"
    }
  }

  # Registered with Cloud Map in BOTH ingress modes. This is deliberately NOT gated on
  # enable_alb: AWS rejects removing service_registries from an existing service, so toggling
  # it would strand the migration (see aws_service_discovery_service.backend below). A task can
  # be in Cloud Map and an ALB target group at once; enable_alb only decides who routes to it.
  service_registries {
    registry_arn   = aws_service_discovery_service.backend[0].arn
    container_name = local.backend_name
    container_port = var.backend_image_port
  }

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }

  # Terraform creates the service pointing at the task definition it knows about,
  # and from then on CI owns which revision is running: the CI Lambda calls
  # UpdateService on every ECR push. Without this the two fight — the Lambda
  # deploys revision :7, the next `terraform apply` sees the service on :7 while
  # its state says :3, and rolls production back to :3 as a side effect of an
  # unrelated change. Nothing reports it, because from Terraform's point of view
  # it just corrected drift.
  #
  # Ownership is therefore stated once, here: Terraform owns the service's shape,
  # CI owns the revision running in it.
  #
  # The precondition lives inside this block because a resource may carry only
  # one lifecycle block. Every pool read in ec2_capacity.tf is total (lookup/try)
  # precisely so that a compute_pool naming a pool that does not exist, or one
  # that exists but is disabled, renders the Fargate value everywhere else and
  # fails HERE — one sentence naming the pool and the fixes, instead of
  # `Error: Invalid index` pointing at a port mapping. Meta-argument: no state,
  # no diff. Needs Terraform >= 1.2, which versions.tf requires.
  lifecycle {
    ignore_changes = [task_definition]

    precondition {
      condition     = module.backend_pool_check.valid["backend"]
      error_message = module.backend_pool_check.message["backend"]
    }
  }

  # ECS rejects a service whose target group has no load balancer attached, and a
  # target group only counts as attached once a listener forwards to it:
  #
  #   InvalidParameterException: The target group with targetGroupArn ... does not
  #   have an associated load balancer.
  #
  # The load_balancer block above references the target group, so Terraform orders
  # this after the target group — but not after the listener, which nothing here
  # mentions. On a first apply the listener waits on ACM certificate validation
  # (minutes), while this service starts immediately, so it reliably lost the race
  # and the apply failed. A second apply then succeeded, because by then the
  # listener existed, which is the signature of a missing edge rather than a
  # broken config.
  #
  # Stated unconditionally: with the ALB off the listener has count = 0 and
  # depending on an empty set is a no-op. The capacity-provider association is
  # here for the same reason in the other direction — the service must never be
  # created before the cluster knows the provider its strategy names — and with
  # no pools it has count = 0 and contributes no edge.
  depends_on = [
    aws_lb_listener.https,
    aws_ecs_cluster_capacity_providers.main,
  ]
}

# Create the Cloud Map service explicitly (instead of letting ECS Service Connect create it).
#
# This is created in BOTH ingress modes, on purpose. AWS does not allow removing
# service_registries from a running ECS service — UpdateService silently ignores the removal,
# the task stays registered, and deleting the Cloud Map service then fails with ResourceInUse.
# Gating this on enable_alb therefore made switching to the ALB an un-appliable dead end. A
# task can belong to Cloud Map and an ALB target group at the same time, so the registration
# simply stays and enable_alb decides only who ROUTES to the task.
#
# count = 1 is kept (rather than dropping count) so the state address stays ...backend[0] and
# existing deployments need no state move.
resource "aws_service_discovery_service" "backend" {
  count = 1

  name = local.backend_name

  # On `terraform destroy`, ECS deregisters instances asynchronously; without this, deleting
  # the service races that and fails with ResourceInUse.
  force_destroy = true

  dns_config {
    namespace_id = aws_service_discovery_private_dns_namespace.local.id

    # An A record is an address and nothing else, so it cannot describe a bridge
    # task: that task has no ENI of its own and sits behind the Docker bridge on
    # a host port chosen at placement time. AWS documents SRV as the supported
    # record type for bridge and host network mode, and ECS registers the host's
    # IPv4 plus the assigned host port.
    #
    # Deliberately NOT a second Cloud Map service. This one keeps its name and
    # its DNS name, so SERVICE_INTERNAL_URL (env_services.tf) and the API Gateway
    # private integration (api_gateway.tf) keep resolving the same thing, and
    # service_registries is never removed from a live ECS service — the
    # constraint recorded on aws_ecs_service.backend above. Only the record set
    # behind the registration differs.
    #
    # ORDER IS LOAD-BEARING: dns_records is a LIST compared positionally. A before
    # SRV. Swapping them replaces every Cloud Map service in the environment.
    dynamic "dns_records" {
      for_each = local.backend_bridge ? [] : [1]
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

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_ecs_task_definition" "backend" {
  # "awsvpc" for Fargate and for a pool that overrode its network mode,
  # "bridge" otherwise. For every existing environment this renders the
  # identical string.
  network_mode             = local.backend_network_mode
  requires_compatibilities = local.backend_pool == null ? ["FARGATE"] : ["EC2"]
  family                   = local.backend_name

  # The 256/512 floors are Fargate task-size rules. On EC2 they have no meaning
  # and are actively harmful: they silently inflate a small task's reservation,
  # which is what bin-packing density is computed from.
  cpu                = local.backend_pool == null ? max(var.backend_cpu, 256) : var.backend_cpu
  memory             = local.backend_pool == null ? max(var.backend_memory, 512) : var.backend_memory
  execution_role_arn = aws_iam_role.backend_task_execution.arn
  task_role_arn      = aws_iam_role.backend_task.arn

  # local.backend_arm64 is the total local from ec2_capacity.tf, never a direct
  # index into local.pools: this expression is evaluated outside the reach of the
  # ECS service's precondition, so a bad pool name must render false here rather
  # than raise `Error: Invalid index`.
  dynamic "runtime_platform" {
    for_each = local.backend_arm64 ? [1] : []
    content {
      cpu_architecture        = "ARM64"
      operating_system_family = "LINUX"
    }
  }

  dynamic "volume" {
    for_each = var.backend_efs_mounts
    content {
      name = volume.value.efs_name
      efs_volume_configuration {
        file_system_id          = var.available_efs[volume.value.efs_name].id
        root_directory          = var.available_efs[volume.value.efs_name].root_directory
        transit_encryption      = "ENABLED"
        transit_encryption_port = 2049
        authorization_config {
          access_point_id = var.available_efs[volume.value.efs_name].access_point_id
        }
      }
    }
  }

  container_definitions = jsonencode(concat(
    local.xray_container,
    [{
      name        = local.backend_name
      command     = var.backend_container_command
      cpu         = local.backend_pool == null ? max(var.backend_cpu, 256) : var.backend_cpu
      memory      = local.backend_pool == null ? max(var.backend_memory, 512) : var.backend_memory
      image       = local.docker_image
      secrets     = local.backend_env_ssm
      environment = concat(local.backend_env, var.backend_env)
      environmentFiles = [
        for file in local.env_files_s3 : {
          value = "arn:aws:s3:::${file.bucket}/${file.key}"
          type  = "s3"
        }
      ]
      essential = true
      mountPoints = [
        for mount in var.backend_efs_mounts : {
          sourceVolume  = mount.efs_name
          containerPath = mount.mount_point
          readOnly      = mount.read_only
        }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.backend.name
          awslogs-stream-prefix = "ecs"
          awslogs-region        = data.aws_region.current.name
        }
      }

      # hostPort == containerPort is an awsvpc construct: under awsvpc the two
      # MUST match, because there is no host-side remapping. Under bridge a fixed
      # hostPort binds that port on the instance, so a second task of the same
      # service cannot place there and the ALB cannot tell two tasks on one host
      # apart. hostPort = 0 asks Docker for an ephemeral port, which is what both
      # target_type = "instance" registration and Cloud Map SRV read.
      portMappings = [{
        protocol      = "tcp"
        containerPort = var.backend_image_port
        hostPort      = local.backend_bridge ? 0 : var.backend_image_port
        name          = local.backend_name
      }]
  }]))

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}


resource "aws_security_group" "backend" {
  name   = "${var.project}_backend_${var.env}"
  vpc_id = var.vpc_id

  # Allow traffic from within VPC (for API Gateway VPC Link and internal services)
  ingress {
    protocol    = "tcp"
    from_port   = var.backend_image_port
    to_port     = var.backend_image_port
    cidr_blocks = [data.aws_vpc.selected.cidr_block]
    description = "Allow traffic from VPC (API Gateway VPC Link)"
  }

  # Allow traffic from the ALB if enabled. The ALB's security group is created in
  # modules/alb, so it is passed in — there is no aws_security_group.alb in this module,
  # and referencing one made the whole ALB path fail to plan.
  dynamic "ingress" {
    for_each = var.enable_alb && var.alb_security_group_id != "" ? [1] : []
    content {
      protocol        = "tcp"
      from_port       = var.backend_image_port
      to_port         = var.backend_image_port
      security_groups = [var.alb_security_group_id]
      description     = "Allow traffic from ALB"
    }
  }

  # Prepare for CloudFront support (commented out until CloudFront is implemented)
  # To enable CloudFront access, uncomment below and add cloudfront_enabled variable
  # dynamic "ingress" {
  #   for_each = var.cloudfront_enabled ? [1] : []
  #   content {
  #     protocol    = "tcp"
  #     from_port   = var.backend_image_port
  #     to_port     = var.backend_image_port
  #     cidr_blocks = data.aws_ip_ranges.cloudfront.cidr_blocks
  #     description = "Allow traffic from AWS CloudFront"
  #   }
  # }

  egress {
    protocol         = "-1"
    from_port        = 0
    to_port          = 0
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  dynamic "egress" {
    for_each = { for mount in var.backend_efs_mounts : mount.efs_name => var.available_efs[mount.efs_name].security_group }
    content {
      protocol        = "tcp"
      from_port       = 2049
      to_port         = 2049
      security_groups = [egress.value]
      description     = "Allow EFS mount access for ${egress.key}"
    }
  }

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_cloudwatch_log_group" "backend" {
  name = "${var.project}_backend_${var.env}"

  retention_in_days = 7

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_s3_bucket" "backend" {
  bucket = module.naming.names["backend_bucket"]
  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_s3_bucket_cors_configuration" "backend" {
  bucket = aws_s3_bucket.backend.id

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    allowed_origins = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3600
  }
}

resource "aws_s3_bucket_ownership_controls" "backend" {
  bucket = aws_s3_bucket.backend.id
  rule {
    object_ownership = "ObjectWriter"
  }
}

resource "aws_s3_bucket_public_access_block" "backend" {
  bucket = aws_s3_bucket.backend.id

  block_public_acls       = !var.backend_bucket_public
  block_public_policy     = !var.backend_bucket_public
  ignore_public_acls      = !var.backend_bucket_public
  restrict_public_buckets = !var.backend_bucket_public
}



resource "aws_s3_bucket_acl" "backend" {
  bucket = aws_s3_bucket.backend.id
  acl    = "private"
  depends_on = [
    aws_s3_bucket_ownership_controls.backend,
    aws_s3_bucket_public_access_block.backend,
  ]
}

resource "aws_iam_role" "backend_task" {
  name               = module.naming.names["backend_task_role"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = module.naming.names["backend_task_role"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role" "backend_task_execution" {
  name               = module.naming.names["backend_task_execution_role"]
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = module.naming.names["backend_task_execution_role"]
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_policy" "full_access_to_backend_bucket" {
  name   = "FullAccessToImagesBucket_${var.project}_${var.env}"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
      {
          "Effect": "Allow",
          "Action": [
              "s3:*"        
           ],
          "Resource": [
            "arn:aws:s3:::${aws_s3_bucket.backend.id}",
            "arn:aws:s3:::${aws_s3_bucket.backend.id}/*"
           ]
      }
  ]
}
EOF

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_policy" "send_emails" {
  name   = "SendSESEmails_${var.project}_${var.env}"
  policy = <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
      {
          "Effect": "Allow",
          "Action": [
              "ses:SendEmail",
              "ses:SendRawEmail"
           ],
          "Resource": "*"
      }
  ]
}
EOF

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "backend_task_execution" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "backend_task_execution_cloudwatch" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role_policy_attachment" "backend_task_cloudwatch" {
  role       = aws_iam_role.backend_task.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role_policy_attachment" "backend_task_backend_bucket" {
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.full_access_to_backend_bucket.arn
}

resource "aws_iam_role_policy_attachment" "backend_task_ses" {
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.send_emails.arn
}

// Add X-Ray permissions to the backend task role
resource "aws_iam_role_policy_attachment" "backend_task_xray" {
  role       = aws_iam_role.backend_task.name
  policy_arn = "arn:aws:iam::aws:policy/AWSXrayWriteOnlyAccess"
}

// EventBridge permissions to allow backend to emit and listen to events
resource "aws_iam_policy" "backend_eventbridge" {
  name = "EventBridgeAccess_${var.project}_backend_${var.env}"
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
    Name        = "EventBridgeAccess_${var.project}_backend_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "backend_task_eventbridge" {
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.backend_eventbridge.arn
}

// SSM IAM access policy for task execution role
resource "aws_iam_role_policy_attachment" "ssm_parameter_access" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
}

// Adding SSM parameter access to task role as well
resource "aws_iam_role_policy_attachment" "ssm_parameter_access_task_role" {
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
}

// Cross-account ECR access policy (only when using cross_account strategy)
data "aws_iam_policy_document" "cross_account_ecr_access" {
  count = var.ecr_strategy == "cross_account" && var.ecr_account_id != "" ? 1 : 0

  # GetAuthorizationToken is account-level
  statement {
    effect = "Allow"
    actions = [
      "ecr:GetAuthorizationToken"
    ]
    resources = ["*"]
  }

  # Repository-specific permissions for cross-account ECR
  statement {
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories"
    ]
    resources = [
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_backend",
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_service_*",
      "arn:aws:ecr:${var.ecr_account_region}:${var.ecr_account_id}:repository/${var.project}_task_*"
    ]
  }
}

resource "aws_iam_policy" "cross_account_ecr_access" {
  count = var.ecr_strategy == "cross_account" && var.ecr_account_id != "" ? 1 : 0

  name        = "${var.project}_cross_account_ecr_access_${var.env}"
  description = "Allow pulling images from cross-account ECR (${var.ecr_account_id})"
  policy      = data.aws_iam_policy_document.cross_account_ecr_access[0].json

  tags = {
    Name        = "${var.project}_cross_account_ecr_access_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role_policy_attachment" "backend_cross_account_ecr" {
  count = var.ecr_strategy == "cross_account" && var.ecr_account_id != "" ? 1 : 0

  role       = aws_iam_role.backend_task_execution.name
  policy_arn = aws_iam_policy.cross_account_ecr_access[0].arn
}

resource "aws_iam_policy" "ssm_parameter_access" {
  name   = "BackendSSMAccessPolicy_${var.project}_${var.env}"
  policy = data.aws_iam_policy_document.ssm_parameter_access.json

  tags = {
    Name        = "BackendSSMAccessPolicy_${var.project}_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

data "aws_iam_policy_document" "ssm_parameter_access" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/backend/*"]
  }
}





resource "aws_iam_role_policy_attachment" "sqs_access" {
  count      = var.sqs_enable == true ? 1 : 0
  role       = aws_iam_role.backend_task.name
  policy_arn = var.sqs_policy_arn
}


# Modify the IAM policy to allow access to multiple files
resource "aws_iam_role_policy" "backend_s3_env" {
  count = length(local.env_files_s3) > 0 ? 1 : 0

  name = "${local.backend_name}-s3-env"
  role = aws_iam_role.backend_task_execution.name

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


// create empty files if they don't exist
resource "null_resource" "create_env_files" {
  for_each = { for file in local.env_files_s3 : "${file.bucket}-${file.key}" => file }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Checking if file exists: ${each.value.bucket}/${each.value.key}"
      touch empty.tmp
      aws s3api head-object --bucket ${each.value.bucket} --key ${each.value.key} || \
      aws s3api put-object --bucket ${each.value.bucket} --key ${each.value.key} --body empty.tmp
      rm empty.tmp
    EOT
  }
}

// remote exec policy
resource "aws_iam_role_policy" "ecs_exec_policy" {
  count = var.backend_remote_access ? 1 : 0

  name = "${var.project}-ecs-exec-policy-${var.env}"
  role = aws_iam_role.backend_task.id
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


// Create custom IAM policy from backend_policy if actions are specified
resource "aws_iam_policy" "backend_custom_policy" {
  count = length(var.backend_policy) > 0 && length(var.backend_policy[0].actions) > 0 ? 1 : 0

  name = "${var.project}_backend_custom_policy_${var.env}"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      for policy in var.backend_policy : {
        Effect   = "Allow"
        Action   = policy.actions
        Resource = policy.resources
      }
    ]
  })

  tags = {
    Name        = local.backend_name
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}

// Attach the custom policy to the backend task role if it exists
resource "aws_iam_role_policy_attachment" "backend_custom_policy_attachment" {
  count = length(var.backend_policy) > 0 && length(var.backend_policy[0].actions) > 0 ? 1 : 0

  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.backend_custom_policy[0].arn
}
