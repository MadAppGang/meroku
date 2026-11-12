# The workload contract: what a capability module (the data bucket and the
# modules that follow it) needs to know about a workload, and what it hands back.
#
# Out: one object per workload with the names of its two IAM roles, the security
# group its traffic leaves from, a plan-known key for that group, its SSM path and
# its EventBridge source. The same shape comes out of modules/ecs_task and
# modules/event_bridge_task as `workload`, so the root can merge all of them into
# one map keyed by workload name.
#
# In: extra_env and extra_secrets, keyed by workload name ("backend" for the
# backend). They are appended to what the container already gets, after it, so an
# environment that sets nothing here renders the task definitions it always has.
#
# None of these outputs reads var.extra_env or var.extra_secrets. That is what lets
# a capability module take the roles from here and give environment variables back
# to the same module call without a dependency cycle.

variable "extra_env" {
  description = "Environment variables meroku's capability modules add, keyed by workload name (\"backend\" or a service name). Appended after the container's own variables."
  type = map(list(object({
    name  = string
    value = string
  })))
  default = {}
}

variable "extra_secrets" {
  description = "Secrets meroku's capability modules add, keyed by workload name (\"backend\" or a service name). Appended after the secrets discovered under the workload's own SSM path. `origin` names where the secret comes from in collision messages; it defaults to valueFrom and never reaches the container."
  type = map(list(object({
    name      = string
    valueFrom = string
    origin    = optional(string)
  })))
  default = {}
}

locals {
  backend_extra_env     = lookup(var.extra_env, "backend", [])
  backend_extra_secrets = [for s in lookup(var.extra_secrets, "backend", []) : { name = s.name, valueFrom = s.valueFrom }]

  services_extra_env = { for k in keys(local.service_names) : k => lookup(var.extra_env, k, []) }
  services_extra_secrets = { for k in keys(local.service_names) : k => [
    for s in lookup(var.extra_secrets, k, []) : { name = s.name, valueFrom = s.valueFrom }
  ] }

  # The same secrets as the collision check sees them: with where each comes from.
  backend_extra_secret_origins = [for s in lookup(var.extra_secrets, "backend", []) : {
    name = s.name, origin = coalesce(s.origin, s.valueFrom)
  }]
  services_extra_secret_origins = { for k in keys(local.service_names) : k => [
    for s in lookup(var.extra_secrets, k, []) : { name = s.name, origin = coalesce(s.origin, s.valueFrom) }
  ] }

  # Exactly what lands in each container definition's `secrets`.
  backend_container_secrets = concat(local.backend_env_ssm, local.backend_extra_secrets)
  services_container_secrets = { for k in keys(local.service_names) : k => concat(
    local.services_env_ssm[k],
    local.services_extra_secrets[k],
  ) }

  # The instance security group of the EC2 pools. In bridge network mode a
  # container has no ENI of its own and its traffic leaves from the instance, so
  # this is the group a bridge-mode workload is seen as. null when no pool exists.
  ecs_instance_security_group_id = try(aws_security_group.ecs_instance[0].id, null)
}

output "backend_workload" {
  description = "The backend's side of the workload contract."
  value = {
    task_role_name      = aws_iam_role.backend_task.name
    execution_role_name = aws_iam_role.backend_task_execution.name
    # aws_security_group.backend is attached only in awsvpc mode (backend.tf);
    # a bridge-mode backend is seen as the instance security group.
    security_group_id = local.backend_bridge ? local.ecs_instance_security_group_id : aws_security_group.backend.id
    sg_key            = local.backend_bridge ? "ecs_instance" : "workload:backend"
    ssm_path          = "/${var.env}/${var.project}/backend"
    event_source      = "${var.project}.backend"
  }
}

output "service_workloads" {
  description = "Each service's side of the workload contract, keyed by service name."
  value = { for k in keys(local.service_names) : k => {
    task_role_name      = aws_iam_role.services_task[k].name
    execution_role_name = aws_iam_role.services_task_execution[k].name
    security_group_id   = local.service_bridge[k] ? local.ecs_instance_security_group_id : aws_security_group.services[k].id
    sg_key              = local.service_bridge[k] ? "ecs_instance" : "workload:${k}"
    ssm_path            = "/${var.env}/${var.project}/${k}"
    event_source        = local.services_event_source[k]
  } }
}

output "backend_bucket_name" {
  description = "Name of the backend bucket, the one the backend gets as AWS_S3_BUCKET."
  value       = aws_s3_bucket.backend.bucket
}

output "backend_bucket_arn" {
  description = "ARN of the backend bucket."
  value       = aws_s3_bucket.backend.arn
}
