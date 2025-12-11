resource "aws_ssm_parameter" "backend_env" {
  name  = "/${var.env}/${var.project}/backend/env"
  type  = "SecureString"
  value = " "

  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }

  tags = {
    Name        = "/${var.env}/${var.project}/backend/env"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}



data "aws_ssm_parameters_by_path" "backend" {
  path      = "/${var.env}/${var.project}/backend"
  recursive = true
  depends_on = [
    aws_ssm_parameter.backend_env
  ]
}



locals {
  backend_env_ssm = [
    for i in range(length(data.aws_ssm_parameters_by_path.backend.names)) : {
      name      = upper(reverse(split("/", data.aws_ssm_parameters_by_path.backend.names[i]))[0])
      valueFrom = data.aws_ssm_parameters_by_path.backend.names[i]
    }
  ]


}

locals {
  # Backend internal domain via Cloud Map service discovery
  backend_internal_domain = "${var.project}_service_${var.env}.${var.private_dns_name}"

  backend_env = concat(
    [
      { "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint },
      { "name" : "PG_DATABASE_USERNAME", "value" : var.db_user },
      { "name" : "PORT", "value" : tostring(var.backend_image_port) },
      { "name" : "PG_DATABASE_NAME", "value" : var.db_name },
      { "name" : "AWS_S3_BUCKET", "value" : "${aws_s3_bucket.backend.bucket}" },
      { "name" : "AWS_REGION", "value" : data.aws_region.current.name },
      { "name" : "URL", "value" : var.api_domain },
      { "name" : "SQS_QUEUE_URL", "value" : var.sqs_queue_url },
      { "name" : "AWS_QUEUE_URL", "value" : var.sqs_queue_url },
      { "name" : "EVENT_BUS_NAME", "value" : "default" },
      { "name" : "EVENT_SOURCE", "value" : "${var.project}.backend" },
      # Domain configuration
      { "name" : "API_DOMAIN", "value" : var.api_domain },
      { "name" : "PRIVATE_DNS_NAMESPACE", "value" : var.private_dns_name },
      { "name" : "BACKEND_INTERNAL_URL", "value" : local.backend_internal_domain },
    ],
    # Add ADOT collector URL when X-Ray is enabled
    var.xray_enabled ? [
      { "name" : "ADOT_COLLECTOR_URL", "value" : "localhost:2000" }
    ] : []
  )
}
