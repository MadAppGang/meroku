locals {
  xray_enabled_container = [{
    name  = "adot-collector"
    image = "public.ecr.aws/aws-observability/aws-otel-collector:latest"
    portMappings = [
      {
        containerPort = 2000
        hostPort      = 2000
        protocol      = "udp"
      },
      {
        containerPort = 4317
        hostPort      = 4317
      },
      {
        containerPort = 4318
        hostPort      = 4318
      },
      {
        containerPort = 55681
        hostPort      = 55681
      }
    ]
    command = [
      "--config=/etc/ecs/container-insights/otel-task-metrics-config.yaml"
    ]
    environment = [
      {
        name  = "AWS_REGION"
        value = data.aws_region.current.name
      }
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = "${var.project}_adot_collector_${var.env}"
        awslogs-region        = data.aws_region.current.name
        awslogs-stream-prefix = "xray"
      }
    }
  }]

  xray_container = var.xray_enabled ? local.xray_enabled_container : []

  app_container_environment = var.xray_enabled ? [
    {
      name  = "ADOT_COLLECTOR_URL"
      value = "localhost:2000"
    }
  ] : []
}

resource "aws_cloudwatch_log_group" "adot_collector" {
  count = var.xray_enabled ? 1 : 0
  name  = "${var.project}_adot_collector_${var.env}"

  retention_in_days = 7

  tags = {
    Name        = "${var.project}_adot_collector_${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    terraform   = "true"
    Application = "${var.project}-${var.env}"
  }
}
