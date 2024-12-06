
locals {
  backend_name = "${var.project}_service_${var.env}"
}


resource "aws_ecs_service" "backend" {
  name                               = local.backend_name
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.backend.arn
  desired_count                      = 1
  deployment_minimum_healthy_percent = 50
  launch_type                        = "FARGATE"
  scheduling_strategy                = "REPLICA"

  network_configuration {
    security_groups  = [aws_security_group.backend.id]
    subnets          = var.subnet_ids
    assign_public_ip = true
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
    terraform = "true"
    env       = var.env
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
        file_system_id = var.available_efs[volume.value.efs_name].id
        root_directory = var.available_efs[volume.value.efs_name].root_directory
        transit_encryption = "ENABLED"
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
        for file in var.env_files_s3 : {
          value = "arn:aws:s3:::${file.bucket}/${file.key}"
          type  = "s3"
        }
      ]
      essential   = true
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
    terraform = "true"
    env       = var.env
  }
}




resource "aws_security_group" "backend" {
  name   = "${var.project}_backend_${var.env}"
  vpc_id = var.vpc_id

  ingress {
    protocol         = "tcp"
    from_port        = var.backend_image_port
    to_port          = var.backend_image_port
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
}

resource "aws_cloudwatch_log_group" "backend" {
  name = "${var.project}_backend_${var.env}"

  retention_in_days = 7

  tags = {
    terraform = "true"
    env       = var.env
  }
}

resource "aws_s3_bucket" "backend" {
  bucket = "${var.project}-backend-${var.env}${var.backend_bucket_postfix}"
  tags = {
    terraform = "true"
    env       = var.env
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
}

resource "aws_iam_role" "backend_task_execution" {
  name               = "${var.project}_backend_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
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
    terraform = "true"
    env       = var.env
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
    terraform = "true"
    env       = var.env
  }
}

resource "aws_iam_role_policy_attachment" "backend_task_execution" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
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


// SSM IAM access policy
resource "aws_iam_role_policy_attachment" "ssm_parameter_access" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = aws_iam_policy.ssm_parameter_access.arn
}

resource "aws_iam_policy" "ssm_parameter_access" {
  name   = "BackendSSMAccessPolicy_${var.project}_${var.env}"
  policy = data.aws_iam_policy_document.ssm_parameter_access.json
}

data "aws_iam_policy_document" "ssm_parameter_access" {
  statement {
    actions   = ["ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"]
    resources = ["arn:aws:ssm:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:parameter/${var.env}/${var.project}/backend/*"]
  }
}


resource "aws_iam_role_policy_attachment" "sqs_access" {
  count      = var.sqs_enable == true ? 1 : 0
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = var.sqs_policy_arn
}

# Modify the IAM policy to allow access to multiple files
resource "aws_iam_role_policy" "backend_s3_env" {
  count = length(var.env_files_s3) > 0 ? 1 : 0
  
  name = "${local.backend_name}-s3-env"
  role = aws_iam_role.backend_task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject"
        ]
        Resource = [
          for file in var.env_files_s3 :
          "arn:aws:s3:::${file.bucket}/${file.key}"
        ]
      }
    ]
  })
}