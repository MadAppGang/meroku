locals {
  aws_account_id = data.aws_caller_identity.current.account_id  
}

variable "env" {
  type = string
}

variable "project" {
  type = string
}

variable "image_bucket_postfix" {
  default = ""
}

variable "lambda_path" {
  type = string
  default = "../../infrastructure/modules/workloads/ci_lambda/main"
}


variable "pgadmin_enabled" {
  type = bool
  default = "false"
}
variable "pgadmin_email" {
  type = string
  default = "admin@madappgang.com"
}


variable "slack_deployment_webhook" {
  default = ""
}

variable "vpc_id" {
  type = string
}

variable "mockoon_enabled" {
  type = bool
  default = false
}



variable "subnet_ids" {
  type = list(string)
}

variable "backend_image_port" {
  default = 8080
  type    = number
}

variable "mockoon_image_port" {
  default = 80
  type    = number
}

variable "private_dns_name" {
  type    = string
}

variable "backend_container_command" {
  type = string
  default = ""
}


variable "backend_health_endpoint" {
  default = "/health/live"
}

variable "zone_id" {
  type = string
}

variable "certificate_arn" {
  type = string
}


variable "domain" {
  type    = string
}

variable "ecr_url" {
  default = ""
}

variable "mockoon_ecr_url" {
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

resource "random_password" "pgadmin" {
  length           = 8
  special          = true
  override_special = "!#$%&*()-_=+[]{}<>:?"
}



resource "aws_ssm_parameter" "pgadmin_password" {
  count = var.pgadmin_enabled ? 1 : 0
  name = "/${var.env}/${var.project}/pgadmin_password"
  type = "SecureString"
  value = random_password.pgadmin.result
}