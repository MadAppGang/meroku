resource "aws_ecs_service" "xray" {
  count                              = var.xray_enabled ? 1 : 0
  name                               = "xray_${var.env}"
  cluster                            = aws_ecs_cluster.main.id
  task_definition                    = aws_ecs_task_definition.xray[0].arn
  desired_count                      = 1
  deployment_minimum_healthy_percent = 50
  launch_type                        = "FARGATE"
  scheduling_strategy                = "REPLICA"

  network_configuration {
    subnets          = var.subnet_ids
    assign_public_ip = true
  }


  tags = {
    terraform = "true"
    env       = var.env
  }

  service_connect_configuration {
    enabled   = true
    namespace = aws_service_discovery_private_dns_namespace.local.name
    //TODO: logs
    service {
      port_name      = "xray_${var.env}"
      discovery_name = "xray_${var.env}"
      client_alias {
        port     = 2000
        dns_name = "xray_${var.env}"
      }
    }
  }

}







resource "aws_ecs_task_definition" "xray" {
  count                    = var.xray_enabled ? 1 : 0
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  family                   = "xray_${var.env}"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.xray_task_execution[0].arn
  task_role_arn            = aws_iam_role.xray_task[0].arn

  container_definitions = jsonencode([{
    name   = "xray_${var.env}"
    cpu    = 32
    memory = 256
    image  = "amazon/aws-xray-daemon"
    environment = [
      { "name" : "AWS_REGION", "value" : tostring(data.aws_region.current.name) },
    ]
    essential = true
    linuxParameters = {
      initProcessEnabled = true
    }
    portMappings = [{
      protocol      = "udp"
      containerPort = 2000
      hostPort      = 2000
    }]
  }])

  tags = {
    terraform = "true"
    env       = var.env
  }
}


resource "aws_cloudwatch_log_group" "xray" {
  count             = var.xray_enabled ? 1 : 0
  name              = "xray_${var.env}"
  retention_in_days = 1
  tags = {
    terraform = "true"
    env       = var.env
  }
}


resource "aws_iam_role" "xray_task" {
  count              = var.xray_enabled ? 1 : 0
  name               = "xray_task_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}

resource "aws_iam_role" "xray_task_execution" {
  count              = var.xray_enabled ? 1 : 0
  name               = "xray_task_execution_${var.env}"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume_role.json
}


resource "aws_iam_role_policy_attachment" "xray_task_execution" {
  count      = var.xray_enabled ? 1 : 0
  role       = aws_iam_role.xray_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy_attachment" "xray_task_cloudwatch" {
  count      = var.xray_enabled ? 1 : 0
  role       = aws_iam_role.xray_task[0].name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchFullAccess"
}

resource "aws_iam_role_policy_attachment" "xray_task_daemon" {
  count      = var.xray_enabled ? 1 : 0
  role       = aws_iam_role.xray_task[0].name
  policy_arn = "arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
}

