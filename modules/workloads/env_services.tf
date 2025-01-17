resource "aws_ssm_parameter" "services_env" {
  for_each = local.service_names

  name  = "/${var.env}/${var.project}/services/${each.key}/env"
  type  = "SecureString"
  value = " "

  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }
}

data "aws_ssm_parameters_by_path" "services" {
  for_each = local.service_names
  
  path      = "/${var.env}/${var.project}/services/${each.key}"
  recursive = true
  depends_on = [
    aws_ssm_parameter.services_env
  ]
}

locals {
  services_env_ssm = {
    for service_name, params in data.aws_ssm_parameters_by_path.services : service_name => [
      for i in range(length(params.names)) : {
        name      = upper(reverse(split("/", params.names[i]))[0])
        valueFrom = params.names[i]
      }
    ]
  }

  services_env = [
    { "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint },
    { "name" : "PG_DATABASE_USERNAME", "value" : var.db_user },
    { "name" : "PG_DATABASE_NAME", "value" : var.db_name },
    { "name" : "AWS_S3_BUCKET", "value" : "${aws_s3_bucket.backend.bucket}" },
    { "name" : "AWS_REGION", "value" : data.aws_region.current.name },
    { "name" : "URL", "value" : var.api_domain },
    { "name" : "SQS_QUEUE_URL", "value" : var.sqs_queue_url },
  ]

}