# One DATABASE_URL per service, at the SSM path modules/workloads already loads
# into the service's task definition: /<env>/<project>/<service>/database_url.
#
# Parameters are keyed by service name only, so moving a service to another
# database is an in-place value update, not a delete and create.

resource "aws_ssm_parameter" "database_url" {
  for_each = toset(var.services)

  name  = "/${var.env}/${var.project}/${each.key}/database_url"
  type  = "SecureString"
  value = var.urls[each.key]

  lifecycle {
    precondition {
      condition     = contains(nonsensitive(keys(var.urls)), each.key)
      error_message = "No database URL was passed for service \"${each.key}\"."
    }
  }

  tags = {
    Name        = "${var.project}-${each.key}-database-url-${var.env}"
    Environment = var.env
    Project     = var.project
    ManagedBy   = "meroku"
    Application = "${var.project}-${var.env}"
  }
}

locals {
  # Service name => "<parameter name>@<version>". The version is unknown at
  # plan whenever the parameter is created or its value changes, so a reader
  # of one token waits for that one write, and only that service's token moves.
  dependency_tokens = {
    for svc in var.services :
    svc => "${aws_ssm_parameter.database_url[svc].name}@${aws_ssm_parameter.database_url[svc].version}"
  }
}
