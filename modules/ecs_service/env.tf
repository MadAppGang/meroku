resource "aws_ssm_parameter" "service_env" {
  name  = "/${var.env}/${var.project}/service/${var.service}/env"
  type  = "SecureString"
  value = " "

  tags = {
    Name        = "${var.project}-${var.service}-env-parameter-${var.env}"
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

data "aws_ssm_parameters_by_path" "service" {
  path      = "/${var.env}/${var.project}/service/${var.service}"
  recursive = true
}

locals {
  service_env_ssm = [
    for i in range(length(data.aws_ssm_parameters_by_path.service.names)) : {
      name      = upper(reverse(split("/", data.aws_ssm_parameters_by_path.service.names[i]))[0])
      valueFrom = data.aws_ssm_parameters_by_path.service.names[i]
    }
  ]
   environment_variables = [
    {
      name  = "SQS_QUEUE_URL"
      value = var.sqs_queue_url
    }
  ] 
}
