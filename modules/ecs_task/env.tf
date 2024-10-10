resource "aws_ssm_parameter" "task_env" {
  name  = "/${var.env}/${var.project}/task/${var.task}/env"
  type  = "SecureString"
  value = " "

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
}
