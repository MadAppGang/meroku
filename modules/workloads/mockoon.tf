

resource "aws_alb_target_group" "mockoon" {
  count                = mockoon_enabled ? 1 : 0
  name                 = "mockoon-tg-${var.env}"
  port                 = var.mockoon_image_port
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

resource "aws_ecs_service" "mockoon" {
  count                              = mockoon_enabled ? 1 : 0
  name                               = "mockoon_service_${var.env}"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.mockoon[0].arn
  desired_count                      = 1
  deployment_minimum_healthy_percent = 50
  launch_type                        = "FARGATE"
  scheduling_strategy                = "REPLICA"

  network_configuration {
    security_groups  = [aws_security_group.mockoon[0].id]
    subnets          = var.subnet_ids
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_alb_target_group.mockoon[0].arn
    container_name   = "${var.project}_mockoon_${var.env}"
    container_port   = var.mockoon_image_port
  }

  service_registries {
    registry_arn = aws_service_discovery_service.mockoon[0].arn
  }

  lifecycle {
    ignore_changes = [task_definition]
  }

  tags = {
    terraform = "true"
    env       = var.env
  }

  depends_on = [
    aws_service_discovery_service.mockoon[0]
  ]
}

resource "aws_ecs_task_definition" "mockoon" {
  count                    = mockoon_enabled ? 1 : 0
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  family                   = "mockoon"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.mockoon_task_execution[0].arn
  task_role_arn            = aws_iam_role.mockoon_task[0].arn

  container_definitions = jsonencode([{
    name   = "${var.project}_mockoon_${var.env}"
    cpu    = 256
    memory = 512
    image  = "${var.env == "dev" ? join("", aws_ecr_repository.mockoon.*.repository_url) : var.mockoon_ecr_url}:latest"
    environment = [
      { "name" : "PORT", "value" : tostring(var.mockoon_image_port) },
    ]
    essential = true
    linuxParameters = {
      initProcessEnabled = true
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.mockoon[0].name
        awslogs-stream-prefix = "ecs"
        awslogs-region        = data.aws_region.current.name
      }
    }

    portMappings = [{
      protocol      = "tcp"
      containerPort = var.mockoon_image_port
      hostPort      = var.mockoon_image_port
    }]
  }])

  tags = {
    terraform = "true"
    env       = var.env
  }
}



resource "aws_security_group" "mockoon" {
  count  = mockoon_enabled ? 1 : 0
  name   = "${var.project}_mockoon_${var.env}"
  vpc_id = var.vpc_id

  ingress {
    protocol         = "tcp"
    from_port        = var.mockoon_image_port
    to_port          = var.mockoon_image_port
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

resource "aws_cloudwatch_log_group" "mockoon" {
  count = mockoon_enabled ? 1 : 0
  name  = "${var.project}_mockoon_${var.env}"

  retention_in_days = 7

  tags = {
    terraform = "true"
    env       = var.env
  }
}


resource "aws_iam_role" "mockoon_task" {
  count              = mockoon_enabled ? 1 : 0
  name               = "${var.project}_mockoon_task_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role" "mockoon_task_execution" {
  count              = mockoon_enabled ? 1 : 0
  name               = "${var.project}_mockoon_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}


resource "aws_iam_role_policy_attachment" "mockoon_task_execution" {
  count      = mockoon_enabled ? 1 : 0
  role       = aws_iam_role.mockoon_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "mockoon_task_cloudwatch" {
  count      = mockoon_enabled ? 1 : 0
  role       = aws_iam_role.mockoon_task[0].name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

