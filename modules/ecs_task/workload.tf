# This scheduled task's side of the workload contract.
# modules/workloads/workload_contract.tf describes the contract; the shape here is
# the same, so the root can merge every workload into one map keyed by name.
#
# extra_env and extra_secrets are appended after the task's own variables and
# secrets. The `workload` output never reads them, which is what lets a capability
# module take the roles from here and hand variables back without a cycle.

variable "extra_env" {
  description = "Environment variables meroku's capability modules add to this task. Appended after the task's own variables."
  type = list(object({
    name  = string
    value = string
  }))
  default = []
}

variable "extra_secrets" {
  description = "Secrets meroku's capability modules add to this task. Appended after the secrets discovered under the task's own SSM path."
  type = list(object({
    name      = string
    valueFrom = string
  }))
  default = []
}

locals {
  # Exactly what lands in the container definition.
  container_environment = concat(local.environment_variables, var.extra_env)
  container_secrets     = concat(local.task_env_ssm, var.extra_secrets)
}

output "workload" {
  description = "This task's side of the workload contract."
  value = {
    task_role_name      = aws_iam_role.task.name
    execution_role_name = aws_iam_role.task_execution.name
    # Tasks always run on Fargate in awsvpc mode, so the task's own group is the
    # one its traffic leaves from.
    security_group_id = aws_security_group.task.id
    sg_key            = "workload:${var.task}"
    ssm_path          = "/${var.env}/${var.project}/task/${var.task}"
    event_source      = "${var.project}.task.${var.task}"
  }
}
