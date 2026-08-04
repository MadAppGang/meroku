variable "project" {
  description = "Project name."
  type        = string
}

variable "env" {
  description = "Environment name."
  type        = string
}

variable "service_names" {
  description = "Names of the named ECS services, exactly as they appear in the project YAML. May contain hyphens."
  type        = list(string)
  default     = []
}

variable "scheduled_task_names" {
  description = "Names of the scheduled (cron) tasks."
  type        = list(string)
  default     = []
}

variable "backend_repo" {
  description = <<-EOT
    Actual ECR repository name backing the backend service, taken from the
    aws_ecr_repository resource. Empty when the backend pulls from another
    account, in which case no local ECR event is ever emitted for it.
  EOT
  type        = string
  default     = ""
}

variable "service_repos" {
  description = <<-EOT
    Service name -> actual ECR repository name. Several services may share one
    repository (ecr_config mode = use_existing); the fan-out is what makes a
    single push deploy all of them. An empty value means the service has no
    repository in this account.
  EOT
  type        = map(string)
  default     = {}
}

variable "task_repos" {
  description = <<-EOT
    Scheduled task name -> ECR repository name in this account. Only the dev
    environment creates these (modules/ecs_task/main.tf); elsewhere the task
    pulls from a cross-account URL and no local ECR event exists, so listing
    the repository would only add an unreachable key.
  EOT
  type        = map(string)
  default     = {}
}

variable "backend_ssm_prefix" {
  description = "SSM path prefix for the backend, without the trailing parameter name."
  type        = string
  default     = ""
}

variable "service_ssm_prefixes" {
  description = "Service name -> SSM path prefix, without the trailing parameter name."
  type        = map(string)
  default     = {}
}

variable "task_ssm_prefixes" {
  description = "Scheduled task name -> SSM path prefix, without the trailing parameter name."
  type        = map(string)
  default     = {}
}

variable "backend_auto_deploy" {
  description = <<-EOT
    May the CI Lambda redeploy the backend on its own, when a new image or a new
    configuration lands? False does not remove the backend from any map: it is
    shipped as a flag so the Lambda can name the reason it did nothing. Manual
    deploys are unaffected.
  EOT
  type        = bool
  default     = true
}

variable "service_auto_deploy" {
  description = <<-EOT
    Service name -> auto-deploy policy. An absent entry means true, which is how
    every project behaved before this setting existed.
  EOT
  type        = map(bool)
  default     = {}
}

variable "task_auto_deploy" {
  description = <<-EOT
    Scheduled task name -> auto-deploy policy. An absent entry means true.

    Note that outside `dev` there is no event that can reach a scheduled task in
    the first place (modules/ecs_task creates its ECR repository only in dev, and
    a scheduled task is deliberately skipped on an SSM change), so `true` there
    enables only the manual path.
  EOT
  type        = map(bool)
  default     = {}
}
