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
  desired_count                      = 1
  deployment_minimum_healthy_percent = 50
  launch_type                        = "FARGATE"
  scheduling_strategy                = "REPLICA"
  enable_ecs_managed_tags            = true
  enable_execute_command             = var.backend_remote_access

  network_configuration {
    security_groups  = [aws_security_group.backend.id]
    subnets          = var.subnet_ids
    assign_public_ip = true
  }

  dynamic "load_balancer" {
    for_each = var.enable_alb ? [1] : []
    content {
      target_group_arn = aws_lb_target_group.backend[0].arn
      container_name   = local.backend_name
      container_port   = var.backend_image_port
    }
  }

  service_connect_configuration {
    enabled   = true
    namespace = aws_service_discovery_private_dns_namespace.local.name

    //TODO: logs
    service {
      port_name      = local.backend_name
      discovery_name = local.backend_name
      client_alias {
        port     = var.backend_image_port
        dns_name = local.backend_name
      }
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

data "aws_service_discovery_service" "backend" {
  namespace_id = aws_service_discovery_private_dns_namespace.local.id
  name         = local.backend_name
}


resource "aws_ecs_task_definition" "backend" {
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  family                   = local.backend_name
  cpu                      = var.xray_enabled ? 512 : 256
  memory                   = var.xray_enabled ? 1024 : 512
  execution_role_arn       = aws_iam_role.backend_task_execution.arn
  task_role_arn            = aws_iam_role.backend_task.arn

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
      cpu         = 256
      memory      = 512
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

      portMappings = [{
        protocol      = "tcp"
        containerPort = var.backend_image_port
        hostPort      = var.backend_image_port
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
    protocol         = "tcp"
    from_port        = var.backend_image_port
    to_port          = var.backend_image_port
    cidr_blocks      = [data.aws_vpc.selected.cidr_block]
    description      = "Allow traffic from VPC (API Gateway VPC Link)"
  }

  # Allow traffic from ALB if enabled
  dynamic "ingress" {
    for_each = var.enable_alb ? [1] : []
    content {
      protocol        = "tcp"
      from_port       = var.backend_image_port
      to_port         = var.backend_image_port
      security_groups = [aws_security_group.alb[0].id]
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
  bucket = "${var.project}-backend-${var.env}${var.backend_bucket_postfix}"
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
  name               = "${var.project}_backend_task_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}_backend_task_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

resource "aws_iam_role" "backend_task_execution" {
  name               = "${var.project}_backend_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json

  tags = {
    Name        = "${var.project}_backend_task_execution_${var.env}"
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
