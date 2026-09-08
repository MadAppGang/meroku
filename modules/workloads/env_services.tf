# Create SSM parameter for each service
resource "aws_ssm_parameter" "services_env" {
  for_each = local.service_names

  name  = "/${var.env}/${var.project}/${each.key}/env"
  type  = "SecureString"
  value = " "

  lifecycle {
    ignore_changes = [
      value,
    ]
  }

  tags = {
    Name        = "/${var.env}/${var.project}/${each.key}/env"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

# Get SSM parameters for each service
data "aws_ssm_parameters_by_path" "services" {
  for_each = local.service_names

  path      = "/${var.env}/${var.project}/${each.key}"
  recursive = true
  depends_on = [
    aws_ssm_parameter.services_env
  ]
}

locals {
  # SSM parameters for each service
  services_env_ssm = {
    for service_name, service in local.service_names : service_name => [
      for i in range(length(data.aws_ssm_parameters_by_path.services[service_name].names)) : {
        name      = upper(reverse(split("/", data.aws_ssm_parameters_by_path.services[service_name].names[i]))[0])
        valueFrom = data.aws_ssm_parameters_by_path.services[service_name].names[i]
      }
    ]
  }

  # Common environment variables for services
  services_env = concat(
    [
      { "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint },
      { "name" : "PG_DATABASE_USERNAME", "value" : var.db_user },
      { "name" : "PG_DATABASE_NAME", "value" : var.db_name },
      { "name" : "AWS_REGION", "value" : data.aws_region.current.name },
      { "name" : "URL", "value" : var.api_domain },
      { "name" : "SQS_QUEUE_URL", "value" : var.sqs_queue_url },
      { "name" : "AWS_QUEUE_URL", "value" : var.sqs_queue_url },
      { "name" : "EVENT_BUS_NAME", "value" : "default" },
      # Domain configuration
      { "name" : "API_DOMAIN", "value" : var.api_domain },
      { "name" : "PRIVATE_DNS_NAMESPACE", "value" : var.private_dns_name },
      { "name" : "BACKEND_INTERNAL_URL", "value" : local.backend_internal_domain },
    ],
    # Add ADOT collector URL for services with X-Ray enabled
    var.xray_enabled ? [
      { "name" : "ADOT_COLLECTOR_URL", "value" : "localhost:2000" }
    ] : []
  )

  # Exactly what lands in the container definition's `environment` — the shared
  # list above, then this service's own env_vars, then the three meroku derives
  # per service. Order is the order it has always been rendered in; jsonencode
  # sorts an object's keys but never a list's elements, so reordering here would
  # register a task-definition revision in every environment for no change.
  #
  # It is a local rather than an inline expression in services.tf because
  # env_secret_check.tf has to compare these names against the discovered secret
  # names, and it has to compare the ones ECS will compare. Two copies of the
  # expression is two copies that can drift, and a check reading the stale copy
  # is worse than no check at all — it reports success on the config that fails.
  services_container_env = {
    for service_name, service in local.service_names : service_name => concat(
      local.services_env,
      [
        for name, value in service.env_vars : {
          name  = name
          value = value
        }
      ],
      [
        { name = "EVENT_SOURCE", value = local.services_event_source[service_name] },
        { name = "SERVICE_INTERNAL_URL", value = local.services_internal_domain[service_name] },
        { name = "SERVICE_NAME", value = service_name },
      ],
    )
  }

  # Per-service EventBridge source name
  services_event_source = {
    for service_name, _ in local.service_names : service_name => "${var.project}.${service_name}"
  }

  # Per-service internal domain via Cloud Map service discovery
  services_internal_domain = {
    for service_name, _ in local.service_names : service_name => "${var.project}_service_${service_name}_${var.env}.${var.private_dns_name}"
  }

  # X-Ray container configuration
  xray_service_container = [
    {
      name              = "xray-daemon"
      image             = "amazon/aws-xray-daemon"
      cpu               = 32
      memoryReservation = 256
      portMappings = [
        {
          containerPort = 2000
          protocol      = "udp"
        }
      ]
    }
  ]
}
