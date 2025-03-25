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

variable "lambda_path" {
  type    = string
  default = "../../infrastructure/modules/workloads/ci_lambda/bootstrap"
}

variable "docker_image" {
  type    = string
  default = ""
}

locals {
  ecr_image    = "${var.env == "dev" ? join("", aws_ecr_repository.backend.*.repository_url) : var.ecr_url}:latest"
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
  default = ["repo:MadAppGang/*"]
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

variable "create_api_domain_record" {
  default = true
}


variable "domain_zone_id" {
  type    = string
  default = ""
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


variable "env_files_s3" {
  type = list(object({
    bucket = string
    key    = string
  }))
  description = "List of S3 environment files to load"
  default     = []
}
locals {
  env_files_s3 = [
    for file in var.env_files_s3 : {
      bucket = "${var.project}-${file.bucket}-${var.env}"
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
    env_files_s3 = optional(list(object({
      bucket = string
      key    = string
    })))
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
