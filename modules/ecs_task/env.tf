resource "aws_ssm_parameter" "task_env" {
  name  = "/${var.env}/${var.project}/task/${var.task}/env"
  type  = "SecureString"
  value = " "

  tags = {
    Name        = "${var.project}-task-${var.task}-env-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }

  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }
}

data "aws_ssm_parameters_by_path" "task" {
  path      = "/${var.env}/${var.project}/task/${var.task}"
  recursive = true
}

locals {
  task_env_ssm = [
    for i in range(length(data.aws_ssm_parameters_by_path.task.names)) : {
      name      = upper(reverse(split("/", data.aws_ssm_parameters_by_path.task.names[i]))[0])
      valueFrom = data.aws_ssm_parameters_by_path.task.names[i]
    }
  ]
  # Default environment variables always present
  default_env_vars = [
    {
      name  = "AWS_REGION"
      value = data.aws_region.current.name
    },
    {
      name  = "SQS_QUEUE_URL"
      value = var.sqs_queue_url
    },
    {
      name  = "EVENT_BUS_NAME"
      value = "default"
    },
    {
      name  = "EVENT_SOURCE"
      value = "${var.project}.task.${var.task}"
    },
    {
      name  = "API_DOMAIN"
      value = var.api_domain
    },
    {
      name  = "PRIVATE_DNS_NAMESPACE"
      value = var.private_dns_name
    },
    {
      name  = "BACKEND_INTERNAL_URL"
      value = var.private_dns_name != "" ? "${var.project}_service_${var.env}.${var.private_dns_name}" : ""
    }
  ]
  # Merge default env vars with custom env vars from YAML
  # Custom vars are appended after defaults (custom can override if same name used)
  environment_variables = concat(local.default_env_vars, var.custom_env_vars)
}
