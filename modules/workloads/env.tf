resource "aws_ssm_parameter" "backend_env" {
  name = "/${var.env}/${var.project}/backend/env"
  type = "SecureString"
  value = " "

  // if we manually change the value, don't rewrite it
  lifecycle {
    ignore_changes = [
      value,
    ]
  }
}

data "aws_ssm_parameters_by_path" "backend" {
  path = "/${var.env}/${var.project}/backend"
  recursive = true
  depends_on = [
    aws_ssm_parameter.backend_env
  ]
}

locals {
  backend_env_ssm = [
    for i in range(length(data.aws_ssm_parameters_by_path.backend.names)) : {
      name      = reverse(split("/", data.aws_ssm_parameters_by_path.backend.names[i]))[0]
      valueFrom = data.aws_ssm_parameters_by_path.backend.names[i]
    }
  ]
}