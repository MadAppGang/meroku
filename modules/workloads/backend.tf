resource "aws_ecr_repository" "backend" {
  name  = "${var.project}_backend"
  count = var.env == "dev" ? 1 : 0

  tags = {
    terraform = "true"
  }
}

resource "aws_ecr_repository_policy" "backend" {
  repository = join("", aws_ecr_repository.backend.*.name)
  policy     = var.ecr_repository_policy
  count      = var.env == "dev" ? 1 : 0
}


resource "aws_alb_target_group" "backend" {
  name                 = "backend-tg-${var.env}"
  port                 = var.backend_image_port
  protocol             = "HTTP"
  vpc_id               = var.vpc_id
  target_type          = "ip"
  deregistration_delay = 30

  health_check {
    path     = "/health/live"
    matcher  = "200"
    interval = 30
  }
}

resource "aws_ecs_service" "backend" {
  name                               = "backend_service_${var.env}"
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

  load_balancer {
    target_group_arn = aws_alb_target_group.backend.arn
    container_name   = "${var.project}_backend_${var.env}"
    container_port   = var.backend_image_port
  }

  service_registries {
    registry_arn = aws_service_discovery_service.backend.arn
  }

  lifecycle {
    ignore_changes = [task_definition]
  }

  tags = {
    terraform = "true"
    env       = var.env
  }

  depends_on = [
    aws_service_discovery_service.backend
  ]
}

resource "aws_ecs_task_definition" "backend" {
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  family                   = "backend"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.backend_task_execution.arn
  task_role_arn            = aws_iam_role.backend_task.arn

  container_definitions = jsonencode([{
    name   = "${var.project}_backend_${var.env}"
    cpu    = 256
    memory = 512
    image  = "${var.env == "dev" ? join("", aws_ecr_repository.backend.*.repository_url) : var.ecr_url}:latest"
    environment = [
      { "name" : "PORT", "value" : tostring(var.backend_image_port) },
      { "name" : "MONGO_URI", "value" : nonsensitive(data.aws_ssm_parameter.mongo_uri.value) },
    ]
    essential = true

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
    }]
  }])

  tags = {
    terraform = "true"
    env       = var.env
  }
}



data "aws_ssm_parameter" "mongo_uri" {
  name = "/${var.env}/ecs/${var.project}-backend/credentials/mongo-uri"
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

resource "aws_s3_bucket" "images" {
  bucket = "${var.project}-images-${var.env}"
  tags = {
    terraform = "true"
    env       = var.env
  }
}


resource "aws_s3_bucket_acl" "images" {
  bucket = aws_s3_bucket.images.id
  acl    = "private"

}

resource "aws_iam_role" "backend_task" {
  name               = "${var.project}_backend_task_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role" "backend_task_execution" {
  name               = "${var.project}_backend_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_policy" "full_access_to_images_bucket" {
  name   = "FullAccessToImagesBucket"
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
            "arn:aws:s3:::${aws_s3_bucket.images.id}",
            "arn:aws:s3:::${aws_s3_bucket.images.id}/*"
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

resource "aws_iam_role_policy_attachment" "backend_task_execution" {
  role       = aws_iam_role.backend_task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "backend_task_cloudwatch" {
  role       = aws_iam_role.backend_task.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role_policy_attachment" "backend_task_images_bucket" {
  role       = aws_iam_role.backend_task.name
  policy_arn = aws_iam_policy.full_access_to_images_bucket.arn
}

