terraform {
  backend "s3" {
    bucket       = ""
    key          = ""
    region       = "ap-southeast-2"
    encrypt      = true
    use_lockfile = true  # Native S3 state locking (Terraform 1.10+)
  }
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
  required_version = ">= 1.10.0"
}

# Stamp every resource in this environment with identifying tags, so the whole
# application can be found in one query (Resource Groups / Tag Editor:
# Application = "myapp-dev").
#
# `region` is pinned from the YAML because the backend already is. Leaving it
# unset gave one value two sources: the S3 backend took the region from this
# config while every resource took it from AWS_REGION / the profile, so a shell
# pointed elsewhere put the state in one region and the infrastructure in
# another, and nothing said so.
#
# Pinning does not create that split, it reveals it: a stack already running
# mismatched will plan a move to the YAML region. checkRegionDrift (app/) warns
# at generation time so it is never discovered from the plan alone.
#
# Resource-level tags win over these, so existing tag blocks keep working.
provider "aws" {
  region = "ap-southeast-2"

  default_tags {
    tags = {
      Application = "myapp-dev"
      Project     = "myapp"
      Environment = "dev"
      ManagedBy   = "meroku"
    }
  }
}

# us-east-1 is needed for two unrelated reasons, so it is declared here rather
# than beside either of them: CloudFront requires its certificate there, and IAM
# and Route53 are global services that AWS hosts in us-east-1 and surfaces only
# through that endpoint. It used to live inside the CloudFront block, which meant
# an environment without CloudFront had no way to reach global resources at all.
#
# Aliased providers do not inherit default_tags, so they are repeated.
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  default_tags {
    tags = {
      Application = "myapp-dev"
      Project     = "myapp"
      Environment = "dev"
      ManagedBy   = "meroku"
    }
  }
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

# ==============================================================================
# Resource Groups — one clickable collection per environment
# ==============================================================================
#
# The Application tag above already makes every resource findable, but a tag on
# its own is only reachable through Tag Editor. A resource group turns the same
# tag into an entry in the console you can open.
#
# This is deliberately NOT myApplications. That service is closed to new
# applications and receives no further features; AWS's own recommended
# replacement is exactly this — tag-based resource groups. myApplications also
# used a different tag (awsApplication, valued with a group ARN), which this
# project never adopted, so there is nothing to migrate.
#
# Two groups are needed, not one. Resource groups are regional, and IAM and
# Route53 are global services whose ARNs carry no region at all
# (arn:aws:iam::123456789012:policy/x, arn:aws:route53:::hostedzone/Z1). AWS
# hosts those in us-east-1 and surfaces them only through that endpoint, so a
# group in the deploy region silently omits every policy and hosted zone this
# environment owns. This is documented behaviour, not a workaround: "To create a
# resource group that contains a global resource, you must configure your AWS
# Management Console to US East (N. Virginia) us-east-1."
#
# Known gap: IAM *roles* are tagged by these modules but never appear in either
# group. The Resource Groups Tagging API does not return iam:role at all —
# verified against a live account where six tagged roles yield zero results while
# six tagged policies yield six. Nothing here can fix that; the roles are still
# tagged and findable through the IAM console and `aws iam list-role-tags`.

resource "aws_resourcegroups_group" "application" {
  name        = "myapp-dev"
  # The API restricts descriptions to [\sa-zA-Z0-9_.-] — no commas, no dashes
  # other than hyphen. It rejects the whole request rather than trimming.
  description = "All myapp dev resources in this region found by the Application tag"

  resource_query {
    query = jsonencode({
      ResourceTypeFilters = ["AWS::AllSupported"]
      TagFilters = [{
        Key    = "Application"
        Values = ["myapp-dev"]
      }]
    })
  }
}

resource "aws_resourcegroups_group" "application_global" {
  provider = aws.us_east_1

  name        = "myapp-dev-global"
  description = "IAM and Route53 for myapp dev - global resources surface via us-east-1"

  resource_query {
    query = jsonencode({
      ResourceTypeFilters = ["AWS::AllSupported"]
      TagFilters = [{
        Key    = "Application"
        Values = ["myapp-dev"]
      }]
    })
  }
}

# Create custom VPC (2 AZs with public subnets only)
module "vpc" {
  source          = "../../infrastructure/modules/vpc"
  project         = "myapp"
  env             = "dev"
  egress_strategy = "public_ip"
}

# Single point of dependency for VPC resources
# All modules reference these locals instead of conditional logic
locals {
  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.subnet_ids

  # Where ECS tasks run, and whether they need a public IPv4 address.
  #
  # These follow egress_strategy: public_ip puts tasks in the public subnets
  # with an address each, while the NAT strategies move them to private subnets
  # behind a NAT Gateway and drop the addresses. Internet-facing resources (the
  # ALB) keep using subnet_ids above, which is always the public set.
  #
  # The default VPC has no private subnets, so it is pinned to public_ip
  # regardless of what the YAML asks for. See ai_docs/EGRESS_COST_MODEL.md.
  task_subnet_ids  = module.vpc.task_subnet_ids
  assign_public_ip = module.vpc.assign_public_ip
}





module "alb" {
  source = "../../infrastructure/modules/alb"
  project = "myapp"
  env = "dev"
  enabled = false
  vpc_id = local.vpc_id
  private_subnets = local.subnet_ids
  # Raise above the app's heartbeat interval to hold SSE / streaming connections open.
  idle_timeout = 60
}



# ============================================================================
# YAML EXTENSIONS - SQS Queues
# Defined in extensions.sqs_queues in YAML config
# ============================================================================
# Dead Letter Queue for jobs
resource "aws_sqs_queue" "ext_jobs_dlq" {
  name = "myapp-dev-jobs-dlq"
  message_retention_seconds = 1209600

  tags = {
    Name        = "myapp-dev-jobs-dlq"
    Project     = "myapp"
    Environment = "dev"
    ManagedBy   = "meroku-extensions"
  }
}

resource "aws_sqs_queue" "ext_jobs" {
  name = "myapp-dev-jobs"
  visibility_timeout_seconds = 30
  message_retention_seconds  = 345600
  max_message_size          = 262144
  delay_seconds             = 0
  receive_wait_time_seconds = 0
  
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.ext_jobs_dlq.arn
    maxReceiveCount     = 3
  })

  tags = {
    Name        = "myapp-dev-jobs"
    Project     = "myapp"
    Environment = "dev"
    ManagedBy   = "meroku-extensions"
  }
}

resource "aws_sqs_queue" "ext_ordered" {
  name = "myapp-dev-ordered.fifo"
  fifo_queue                  = true
  content_based_deduplication = true
  visibility_timeout_seconds = 30
  message_retention_seconds  = 345600
  max_message_size          = 262144
  delay_seconds             = 0
  receive_wait_time_seconds = 0
  
  tags = {
    Name        = "myapp-dev-ordered"
    Project     = "myapp"
    Environment = "dev"
    ManagedBy   = "meroku-extensions"
  }
}

module "workloads" {
  source = "../../infrastructure/modules/workloads"
  project    = "myapp"
  env        = "dev" 
  private_dns_name = "myapp_dev.private"

  enable_custom_domain = false
  api_domain = ""
  create_api_domain_record = false
  domain = ""
  vpc_id     = local.vpc_id
  # ECS tasks and the API Gateway VPC links follow the egress strategy. The ALB
  # is a separate module and stays on the public subnets.
  subnet_ids       = local.task_subnet_ids
  assign_public_ip = local.assign_public_ip
  slack_deployment_webhook = ""
  backend_bucket_postfix = ""
  backend_bucket_public = false
  backend_health_endpoint = ""
  backend_remote_access = true
  docker_image = ""
  setup_FCM_SNS = false
  backend_image_port = 8080
    ecr_strategy = ""
    
  ecr_trusted_accounts = []
  ecr_image_events = false
  is_prod = false
  github_oidc_enabled = false
  github_oidc_create_provider = true
  github_push_external_image = false
  github_subjects = []
  xray_enabled = false
  pgadmin_enabled = false
  pgadmin_email = ""
  # Backend environment variables merged from:
  # 1. YAML backend_env_variables
  # 2. Custom pre-module outputs (if exists)
  # 3. YAML extensions (SNS topics, SQS queues with add_to_backend_env)
  backend_env = concat(
    [],
    [{ name = "JOBS_QUEUE_URL", value = aws_sqs_queue.ext_jobs.url }]
  )

  # May the CI/CD Lambda redeploy the backend on its own when a new image or a
  # new configuration lands? false does not remove anything from the Lambda's
  # maps: the flag travels to it as data so a push still logs "auto_deploy is
  # disabled for backend" instead of nothing. Manual deploys are unaffected.
  # A target without its own value follows the environment's auto_deploy.
  auto_deploy = false
  backend_auto_deploy = false

  # Backend scaling configuration
  backend_cpu = 256
  backend_memory = 512
  backend_desired_count = 1

  # Backend autoscaling configuration
  backend_autoscaling_enabled = false
  backend_autoscaling_min_capacity = 1
  backend_autoscaling_max_capacity = 10
  backend_autoscaling_target_cpu = 70
  backend_autoscaling_target_memory = 80

  # ---------------------------------------------------------------------------
  # EC2 compute pools (schema v26).
  #
  # Absent by default: an environment with no `compute:` block renders only
  # backend_runtime = "fargate" here, which is the variable's own default, and
  # plans identically to v25.
  #
  # Every user-controlled string below goes through an escaping helper, because
  # HCL2 opens an interpolation on `${` and a directive on `%{` inside quoted
  # strings AND heredocs alike -- so an unescaped value out of YAML is Terraform
  # syntax rather than data. hclString quotes and escapes a scalar, hclArray
  # does the same for every string inside a list, and hclEscape handles the
  # heredoc body, where backslash escapes do not apply and only the two openers
  # may be doubled.
  #
  # network_mode is rendered unconditionally -- it is the one field where
  # silence would be dangerous, because a reader of this file must be able to
  # see which mode a pool is in without knowing what the default is. Three
  # layers hold the same default (the Go structs, the template helper, and
  # optional(string, "bridge") in variables.tf) and a template test pins that
  # they agree.
  #
  # min_size and on_demand_base are gated on `exists`, not on `default` and not
  # on a bare `if`: 0 is meaningful for both -- min_size: 0 is a pool that
  # scales to nothing -- and a truthiness test would silently drop the argument
  # and let the variable default take over instead.
  # ---------------------------------------------------------------------------
  backend_runtime = "fargate"
            # The ALB serves api_domain — the same hostname API Gateway serves — so enabling it does
  # not change the public API URL. backend_alb_domain_name is an optional EXTRA hostname,
  # not the switch that turns the ALB on; requiring it was what made alb.enabled inert.
  #
  # These two are passed in BOTH modes (they resolve to "" when the ALB is off) so the
  # dependency on module.alb survives being disabled. Only enable_alb decides behaviour.
  # Gating the references themselves is what let Terraform destroy the ALB security group
  # while the backend rule still pointed at it.
  alb_arn = module.alb.alb_arn
  alb_security_group_id = module.alb.alb_security_group_id
  enable_alb = false

  
  backend_policy = []
      # Variables and secrets the capability modules add per workload (local.workload_extra_env).
  extra_env     = local.workload_extra_env
  extra_secrets = local.workload_extra_secrets
  # EVENT_BUS_NAME: the project bus when event_bus.enabled, otherwise "default".
  event_bus_name = module.events.bus_name
}

# Every workload of the environment, keyed by name, with the role names, security
# group, SSM path and event source the capability modules grant against. Names are
# unique across all kinds (app/workload_names.go), so the merge never overwrites.
locals {
  workload_roles = merge(
    { backend = module.workloads.backend_workload },
    module.workloads.service_workloads,
    {
    },
  )

  # What each workload's container gets on top of its own variables and secrets.
  workload_extra_env = { for name in keys(local.workload_roles) : name => concat(
    module.data_bucket.environment[name],
    module.events.environment[name],
    module.kms_keys.environment[name],
  ) }
  workload_extra_secrets = { for name in keys(local.workload_roles) : name => concat(
    module.vaults.secrets[name],
  ) }
}

# The private data bucket: a folder per workload, plus the shared folders listed
# under data_bucket.shared. See ai_docs/DATA_BUCKET.md.
module "data_bucket" {
  source  = "../../infrastructure/modules/data_bucket"
  project = "myapp"
  env     = "dev"

  workloads              = local.workload_roles
  shared                 = []
  own_backend_bucket_env = []
  backend_bucket_name    = module.workloads.backend_bucket_name
  backend_bucket_arn     = module.workloads.backend_bucket_arn
}

# Shared SSM vaults, /dev/myapp/vault/<name>/: every secret a vault
# declares in the YAML becomes a secret of each workload connected to it. The
# values live only in SSM, set from the web UI. See ai_docs/VAULTS.md.
module "vaults" {
  source  = "../../infrastructure/modules/vaults"
  project = "myapp"
  env     = "dev"

  workloads   = local.workload_roles
  vaults      = {}
  connections = []
}

# KMS signing and HMAC keys: a key per kms_keys entry, retired ones included.
# A connected workload may sign, verify or both, and gets KMS_<NAME>_KEY_ARN,
# plus KMS_<NAME>_PUBLIC_KEY for a signing key. See ai_docs/KMS_KEYS.md.
module "kms_keys" {
  source  = "../../infrastructure/modules/kms_keys"
  project = "myapp"
  env     = "dev"

  workloads   = local.workload_roles
  keys        = {}
  connections = []
}

# Events: the project bus (event_bus.enabled), a queue per workload that
# subscribes, and PutEvents for each events.produces entry. Without an entry a
# workload cannot publish. See ai_docs/EVENTS.md.
module "events" {
  source  = "../../infrastructure/modules/events"
  project = "myapp"
  env     = "dev"

  bus_enabled = true
  workloads   = local.workload_roles
  consumers   = [{"max_receive_count":5,"name":"backend","subscriptions":[{"bus":"project","detail_type":"order.created","detail_type_prefix":"","key":"order-created","source":""}],"visibility_timeout":30}]
  producers   = []
}


# Email domain configuration (Schema v17: Multi-domain support)





# ============================================================================
# YAML EXTENSIONS - SQS to SNS Subscriptions
# Subscribe SQS queues to SNS topics for fan-out pattern
# ============================================================================





output "backend_ecr_repo_url" {
  value = module.workloads.backend_ecr_repo_url
}

output "account_id" {
  value = module.workloads.account_id
}

output "region" {
  value = data.aws_region.current.region
}

output "backend_task_role_name" {
  value = module.workloads.backend_task_role_name
}

output "backend_cloud_map_arn" {
  description = "value of the backend service discovery ARN"
  value       = module.workloads.backend_cloud_map_arn
}



