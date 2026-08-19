locals {
  aws_account_id = data.aws_caller_identity.current.account_id
}

variable "env" {
  type = string
}

variable "project" {
  type = string
}

variable "backend_bucket_postfix" {
  default = ""
}


variable "backend_bucket_public" {
  default = true
}

# Deprecated and unused. lambda.tf builds the CI Lambda itself at apply time
# (null_resource.build_ci_lambda -> ci_lambda/.build/${var.env}/ci_lambda.zip),
# so nothing reads this path. env/main.hbs no longer emits it.
#
# It must stay declared: passing an undeclared variable is a hard Terraform
# error, and every env/*/main.tf generated before this change still sets it.
# Remove it only once those files have been regenerated.
variable "lambda_path" {
  description = "Deprecated, unused. The CI Lambda artifact is built by lambda.tf at apply time."
  type        = string
  default     = "../../infrastructure/modules/workloads/ci_lambda/bootstrap"
}

variable "docker_image" {
  type    = string
  default = ""
}

locals {
  ecr_image    = "${var.ecr_strategy == "local" ? join("", aws_ecr_repository.backend.*.repository_url) : var.ecr_url}:latest"
  docker_image = var.docker_image != "" ? var.docker_image : local.ecr_image
}

variable "xray_enabled" {
  description = "Whether to enable X-Ray daemon container"
  type        = bool
  default     = false
}

variable "slack_deployment_webhook" {
  default = ""
}

variable "vpc_id" {
  type = string
}

variable "mockoon_enabled" {
  type    = bool
  default = false
}

variable "subnet_ids" {
  type = list(string)
}

variable "github_subjects" {
  type    = list(string)
  default = ["repo:MadAppGang/*:*"]
}

variable "github_oidc_enabled" {
  type    = bool
  default = false
}
variable "backend_image_port" {
  default = 8080
  type    = number
}

variable "backend_env" {
  default = [
    { "name" : "BACKEND_TEST", "value" : "TEST" },
  ]
}

variable "private_dns_name" {
  type = string
}

variable "backend_container_command" {
  type    = list(string)
  default = []
}


variable "backend_health_endpoint" {
  default = "/health/live"
}


variable "subdomains_certificate_arn" {
  type    = string
  default = ""
}

variable "api_certificate_arn" {
  type    = string
  default = ""
}

variable "api_domain" {
  type = string
}

variable "domain" {
  default = ""
}

# Env-resolved domain from the domain module ("<env>.<zone>" or "<zone>", per
# add_env_domain_prefix). Use this — never "${var.env}.${var.domain}" — whenever a
# hostname has to sit inside the environment's Route53 zone and wildcard certificate.
variable "env_domain" {
  description = "Env-resolved domain name (module.domain.domain_name)"
  type        = string
  default     = ""
}

variable "create_api_domain_record" {
  default = true
}


variable "domain_zone_id" {
  type    = string
  default = ""
}

variable "enable_custom_domain" {
  description = "Enable API Gateway custom domain resources (from domain module)"
  type        = bool
  default     = false
}


variable "ecr_strategy" {
  description = "ECR repository strategy: 'local' to create ECR in this account, 'cross_account' to pull from another account"
  type        = string
  default     = "local"

  validation {
    condition     = contains(["local", "cross_account"], var.ecr_strategy)
    error_message = "ecr_strategy must be either 'local' or 'cross_account'"
  }
}

variable "ecr_account_id" {
  description = "AWS account ID for cross-account ECR access (required when ecr_strategy is 'cross_account')"
  type        = string
  default     = ""
}

variable "ecr_account_region" {
  description = "AWS region for cross-account ECR access (required when ecr_strategy is 'cross_account')"
  type        = string
  default     = ""
}

variable "ecr_trusted_accounts" {
  description = "List of AWS accounts allowed to pull from this environment's ECR repositories"
  type = list(object({
    account_id = string
    env        = string
    region     = string
  }))
  default = []
}

variable "ecr_url" {
  default = ""
}

variable "db_endpoint" {
  default = ""
}

variable "db_user" {
  default = ""
}

variable "db_name" {
  default = ""
}

variable "setup_FCM_SNS" {
  default = false
}

variable "sqs_queue_url" {
  default = ""
}

variable "sqs_policy_arn" {
  default = ""
}

variable "sqs_enable" {
  default = false
}

variable "ses_enabled" {
  description = "Enable SES email sending permissions for backend and services"
  type        = bool
  default     = false
}

variable "env_files_s3" {
  type = list(object({
    bucket = string
    key    = string
  }))
  description = "List of S3 environment files to load"
  default     = []
}
locals {
  # Use bucket name as-is - user specifies the full bucket name in YAML
  env_files_s3 = [
    for file in var.env_files_s3 : {
      bucket = file.bucket
      key    = file.key
    }
  ]
}

variable "ecr_lifecycle_policy" {
  type    = string
  default = <<EOF
{
    "rules": [
        {
            "rulePriority": 1,
            "description": "Delete untagged images",
            "selection": {
                "tagStatus": "untagged",
                "countType": "imageCountMoreThan",
                "countNumber": 1
            },
            "action": {
                "type": "expire"
            }
        },
        {
            "rulePriority": 2,
            "description": "Keep no more than 10 recent images",
            "selection": {
                "tagStatus": "any",
                "countType": "imageCountMoreThan",
                "countNumber": 10
            },
            "action": {
                "type": "expire"
            }
        }
    ]
}
EOF
}

variable "available_efs" {
  description = "Map of available EFS resources"
  type = map(object({
    id              = string
    access_point_id = string
    dns_name        = string
    security_group  = string
    root_directory  = string
  }))
  default = {}
}


variable "backend_efs_mounts" {
  description = "List of EFS mounts for backend service"
  type = list(object({
    efs_name    = string                # Name of EFS to mount
    mount_point = optional(string, "/") # Container mount path
    read_only   = optional(bool, false)
  }))
  default = []
}



// domain name for expose backend with ALB
variable "backend_alb_domain_name" {
  default = ""
}

variable "alb_arn" {
  default = ""
}

# Security group of the ALB (created in modules/alb). The backend's security group uses
# it to allow ingress only from the load balancer.
variable "alb_security_group_id" {
  description = "Security group ID of the ALB, used as the source for backend ingress"
  type        = string
  default     = ""
}

variable "enable_alb" {
  description = "Enable ALB"
  type        = bool
  default     = false
}


variable "backend_remote_access" {
  type    = bool
  default = false
}

variable "services" {
  type = list(object({
    name              = string
    remote_access     = optional(bool, false)
    container_port    = optional(number, 3000)
    container_command = optional(list(string), [])
    host_port         = optional(number, 3000)
    cpu               = optional(number, 256)
    memory            = optional(number, 512)
    xray_enabled      = optional(bool, false)
    docker_image      = optional(string, "")
    env_vars          = optional(map(string), { "name" : "SERVICE_TEST", "value" : "PASSED" })
    essential         = optional(bool, true)
    desired_count     = optional(number, 1)
    # May the CI Lambda redeploy this service on its own? See
    # var.backend_auto_deploy for what false does and does not do. Absent means
    # true, which is how every project behaved before this setting existed.
    auto_deploy = optional(bool, true)
    # Where this service runs. "fargate" keeps the literal launch_type it has
    # today, byte-identical; "ec2" places it on compute_pool via a capacity
    # provider strategy.
    #
    # These two lines are load-bearing rather than decorative: without them the
    # per-service fields travel inside {{{array services}}} (env/main.hbs) and
    # are dropped without a word by Terraform's object-type conversion — the
    # exact failure mode app/autodeploy_template_test.go was written to catch.
    runtime      = optional(string, "fargate")
    compute_pool = optional(string, "")
    env_files_s3 = optional(list(object({
      bucket = string
      key    = string
    })))
    ecr_config = optional(object({
      mode                = optional(string, "create_ecr")
      repository_uri      = optional(string, "")
      source_service_name = optional(string, "")
      source_service_type = optional(string, "")
    }))
  }))
  default = []
}


locals {
  services_env_files_s3 = {
    for service in var.services :
    service.name => [
      for file in coalesce(service.env_files_s3, []) : {
        bucket = file.bucket
        key    = file.key
      }
    ]
  }
}

variable "backend_policy" {
  type = list(object({
    actions   = list(string)
    resources = list(string)
  }))
  default = [
    {
      actions   = []
      resources = ["*"]
    }
  ]
  description = "Custom IAM policy for the backend task"
}

# Backend service scaling configuration
variable "backend_cpu" {
  description = "CPU units for the backend task (256, 512, 1024, 2048, 4096)"
  type        = number
  default     = 256
}

variable "backend_memory" {
  description = "Memory for the backend task in MB"
  type        = number
  default     = 512
}

variable "backend_desired_count" {
  description = "Desired number of backend tasks"
  type        = number
  default     = 1
}

variable "backend_autoscaling_enabled" {
  description = "Enable autoscaling for backend service"
  type        = bool
  default     = false
}

variable "backend_autoscaling_min_capacity" {
  description = "Minimum number of tasks for autoscaling"
  type        = number
  default     = 1
}

variable "backend_autoscaling_max_capacity" {
  description = "Maximum number of tasks for autoscaling"
  type        = number
  default     = 10
}

variable "backend_autoscaling_target_cpu" {
  description = "Target CPU utilization percentage for autoscaling"
  type        = number
  default     = 70
}

# NOTE for existing deployments: until these two were wired into
# backend_autoscaling.tf, the memory policy hardcoded 75.0 while this variable
# documented 80. The first apply after that fix moves the memory target
# 75 -> 80 on any environment with backend autoscaling enabled and no explicit
# value set. That is the documented default taking effect, not a new default.
variable "backend_autoscaling_target_memory" {
  description = "Target memory utilization percentage for autoscaling"
  type        = number
  default     = 80
}

variable "scheduled_task_names" {
  description = "List of scheduled task names for Lambda CI/CD auto-deployment"
  type        = list(string)
  default     = []
}

variable "backend_auto_deploy" {
  description = <<-EOT
    May the CI/CD Lambda redeploy the backend on its own when a new image or a
    new configuration lands?

    false does NOT remove the backend from any map and does NOT remove its
    repository from the ECR event rule. The flag travels to the Lambda as data,
    the Lambda is still invoked, and it logs "auto_deploy is disabled for
    backend" instead of doing nothing silently or claiming the repository is
    unmapped. Manual deploys (detail-type DEPLOY / SERVICE_DEPLOY) are
    unaffected: this governs automatic triggers only.

    Default true, because that is what every project did before this setting
    existed. meroku's YAML migration writes the value explicitly, defaulting to
    true in dev and false everywhere else.
  EOT
  type        = bool
  default     = true
}

variable "scheduled_task_auto_deploy" {
  description = <<-EOT
    Scheduled task name -> auto-deploy policy. An absent entry means true.

    Outside `dev` no automatic trigger reaches a scheduled task at all
    (modules/ecs_task creates the task's ECR repository only in dev, and an SSM
    change deliberately never redeploys a task), so `true` there enables only the
    manual path.
  EOT
  type        = map(bool)
  default     = {}
}

variable "pgadmin_enabled" {
  description = "Enable pgAdmin deployment"
  type        = bool
  default     = false
}

variable "pgadmin_email" {
  description = "Default email for pgAdmin login"
  type        = string
  default     = "admin@example.com"
}

# ---------------------------------------------------------------------------
# EC2 compute pools. See ec2_capacity.tf.
# ---------------------------------------------------------------------------

variable "backend_runtime" {
  description = "Where the backend service runs: \"fargate\" (default) or \"ec2\" on a compute pool."
  type        = string
  default     = "fargate"

  validation {
    condition     = contains(["fargate", "ec2"], var.backend_runtime)
    error_message = "backend_runtime must be \"fargate\" or \"ec2\"."
  }
}

variable "backend_compute_pool" {
  description = "Name of the compute pool the backend runs on. Required when backend_runtime is \"ec2\", ignored otherwise."
  type        = string
  default     = ""
}

variable "compute_pools" {
  description = <<-EOT
    ASG-backed ECS capacity providers. Absent (the default) means no EC2
    capacity and no diff — every resource in ec2_capacity.tf is guarded on this
    list being non-empty after the `enabled` filter.

    network_mode defaults to "bridge": the task shares the container instance's
    primary ENI, which already has a public IP, so egress works with no NAT
    gateway. "awsvpc" is a per-pool override that generate-time validation
    refuses unless the pool asserts an egress path.

    on_demand_base counts INSTANCES, not tasks — it renders as
    on_demand_base_capacity inside instances_distribution.

    `assume_egress` is a generate-time assertion only and is deliberately NOT
    declared here: nothing under modules/ reads it.
  EOT

  type = list(object({
    name            = string
    enabled         = optional(bool, true)
    instance_types  = list(string)
    capacity_type   = optional(string, "on_demand")
    on_demand_base  = optional(number, 0)
    min_size        = optional(number, 1)
    max_size        = optional(number, 6)
    target_capacity = optional(number, 100)
    # bridge is the default here, in the template, and in the Go structs —
    # three defaults that must agree, because a pool that omits the key must
    # render and plan identically whichever layer supplied the value.
    network_mode    = optional(string, "bridge")
    ami_family      = optional(string, "al2023")
    ami_id          = optional(string, "")
    root_volume_gb  = optional(number, 30)
    user_data_extra = optional(string, "")
    extra_volumes = optional(list(object({
      device_name = string
      size_gb     = number
      type        = optional(string, "gp3")
    })), [])
    instance_policies = optional(list(object({
      actions   = list(string)
      resources = list(string)
    })), [])
  }))

  default = []

  # optional(..., default) is what lets local.pools read p.enabled and
  # p.network_mode directly: Terraform substitutes the default during type
  # conversion, so the attribute is present and non-null on every element,
  # whatever the YAML omitted. With a bare optional(bool) the `if p.enabled`
  # filter would fail on a null condition.
  validation {
    condition     = alltrue([for p in var.compute_pools : contains(["bridge", "awsvpc"], p.network_mode)])
    error_message = "compute_pools[*].network_mode must be \"bridge\" or \"awsvpc\"."
  }
}
