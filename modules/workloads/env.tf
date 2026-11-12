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



locals {
  # One entry per service path in var.ssm_dependencies: always "", but unknown
  # while that service's token is unknown, that is while a parameter another
  # module writes under the path is created or changes value in this plan. An
  # unknown path defers that one by-path read to apply, after the write, so the
  # new value reaches that service's task definition in the same apply and no
  # other service is touched. A service without a token reads at plan, as
  # before.
  ssm_path_gate = { for svc, token in var.ssm_dependencies : svc => substr(token, 0, 0) }
}

data "aws_ssm_parameters_by_path" "backend" {
  path      = "/${var.env}/${var.project}/backend${lookup(local.ssm_path_gate, "backend", "")}"
  recursive = true
  depends_on = [
    aws_ssm_parameter.backend_env
  ]
}



locals {
  # The /env parameter's name is a configured value, so ENV is in the list
  # whatever the read returns. Discovered names come first, so where /env is
  # already discovered the list is unchanged. Parameters named in
  # ssm_exclude_names are deleted in this same apply; the by-path read still
  # lists them, and a secret pointing at a deleted parameter would stop the
  # task from starting.
  backend_ssm_names = [
    for n in distinct(concat(data.aws_ssm_parameters_by_path.backend.names, [aws_ssm_parameter.backend_env.name])) : n
    if !contains(var.ssm_exclude_names, n)
  ]

  backend_env_ssm = [
    for n in local.backend_ssm_names : {
      name      = upper(reverse(split("/", n))[0])
      valueFrom = n
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
      { "name" : "AWS_REGION", "value" : data.aws_region.current.region },
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

  # Exactly what lands in the container definition's `environment`. A local
  # rather than an inline concat in backend.tf for the same reason as
  # local.services_container_env (env_services.tf): env_secret_check.tf compares
  # these names against the discovered secret names and must compare the ones
  # ECS will compare, and a second copy of the expression is a copy that can
  # drift into reporting success on the config that fails.
  backend_container_env = concat(local.backend_env, var.backend_env)
}
