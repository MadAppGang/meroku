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
  services_env = [
    { "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint },
    { "name" : "PG_DATABASE_USERNAME", "value" : var.db_user },
    { "name" : "PG_DATABASE_NAME", "value" : var.db_name },
    { "name" : "AWS_REGION", "value" : data.aws_region.current.name },
    { "name" : "URL", "value" : var.api_domain },
    { "name" : "SQS_QUEUE_URL", "value" : var.sqs_queue_url },
    { "name" : "AWS_QUEUE_URL", "value" : var.sqs_queue_url },
  ]

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
