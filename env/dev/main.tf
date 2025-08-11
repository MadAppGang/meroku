terraform {
  backend "s3" {
    bucket = "instagram-terraform-state-dev"
    key    = "terraform.tfstate"
    region = "ap-southeast-2"
  }
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.2.6"
}

data "aws_vpc" "default" {
  default = true
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}
data "aws_subnets" "all" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# Locals removed - using direct handlebar resolution in modules
module "domain" {
  source = "../../modules/domain"
  project = "instagram"
  domain_zone = "instagram.madappgang.com.au"
  env = "dev"
  create_domain_zone = false
  api_domain_prefix = ""
  add_env_domain_prefix = true
}


module "postgres" {
  source = "../../modules/postgres"
  project = "instagram"
  env = "dev"
  vpc_id     = data.aws_vpc.default.id
  subnet_ids = data.aws_subnets.all.ids
  db_name = "instagram"
  username = "dbadmin"
  public_access = true
  engine_version = "14"
  aurora = true
  min_capacity = 0.5
  
  max_capacity = 2
  
}


module "efs" {
  source = "../../modules/efs"
  project = "instagram"
  env = "dev"
  vpc_id = data.aws_vpc.default.id
  private_subnets = data.aws_subnets.all.ids
  efs_configs = [{"name":"uploads"},{"name":"static"}]
}
module "alb" {
  source = "../../modules/alb"
  project = "instagram"
  env = "dev"
  vpc_id = data.aws_vpc.default.id
  private_subnets = data.aws_subnets.all.ids
}
module "workloads" {
  source = "../../modules/workloads"
  project    = "instagram"
  env        = "dev" 
  private_dns_name = "instagram_dev.private"
  api_domain = module.domain.api_domain_name
  create_api_domain_record = false
    domain = "instagram.madappgang.com.au"
  vpc_id     = data.aws_vpc.default.id
  subnet_ids = data.aws_subnets.all.ids
  lambda_path = "../../modules/workloads/ci_lambda/bootstrap"
  slack_deployment_webhook = ""
  backend_bucket_postfix = ""
  backend_bucket_public = false
  backend_health_endpoint = ""
  backend_remote_access = true
  docker_image = ""
  setup_FCM_SNS = false
  backend_image_port = 8080

  db_endpoint = module.postgres.endpoint
  db_user = "dbadmin"
  db_name = "instagram"
  
  subdomains_certificate_arn = module.domain.subdomains_certificate_arn
  api_certificate_arn = module.domain.api_certificate_arn
  domain_zone_id = module.domain.zone_id
  
  github_oidc_enabled = false
  github_subjects = []
  xray_enabled = false
  pgadmin_enabled = false
  pgadmin_email = "admin@admin.com"
  backend_env = [
    { "name" : "env1", "value" : "value1" }
  ]
  
  # Backend scaling configuration
  backend_cpu = 512
  
  backend_memory = 1024
  
  backend_desired_count = 2
  
  # Backend autoscaling configuration
  backend_autoscaling_enabled = true
  
  backend_autoscaling_min_capacity = 2
  
  backend_autoscaling_max_capacity = 10
            
  backend_alb_domain_name = "alb.instagram.madappgang.com.au"
  alb_arn = module.alb.alb_arn
  enable_alb = true
  
  services = [{"container_port":3000,"cpu":256,"desired_count":3,"env_vars":{"SERVICE_TEST":"PASSED"},"host_port":3000,"memory":512,"name":"service1","remote_access":true,"xray_enabled":false}] 
  backend_policy = [
  ]
}
module "cognito" {
  source                  = "../../modules/cognito"
  project                 = "instagram"
  env                     = "dev"
  enable_web_client       = "false"
  enable_dashboard_client = false
  dashboard_callback_urls = ["https://jwt.io"]

  enable_user_pool_domain = false
  user_pool_domain_prefix = ""

  allow_backend_task_to_confirm_signup = false
  auto_verified_attributes = ["email"]
  backend_task_role_name  = module.workloads.backend_task_role_name
}
# scheduled task
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/setting-up.html#setting-up-execution-role
module "schedule_task_task1" {
  source = "../../modules/ecs_task"
  project = "instagram"
  env = "dev"
  task = "task1"
    # https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html?icmpid=docs_console_unmapped#rate-based
  schedule = "rate(1 minutes)"
  subnet_ids = data.aws_subnets.all.ids
  vpc_id     = data.aws_vpc.default.id
  cluster = module.workloads.ecr_cluster.arn
  allow_public_access = false
  
}
# scheduled task
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/setting-up.html#setting-up-execution-role
module "schedule_task_task2" {
  source = "../../modules/ecs_task"
  project = "instagram"
  env = "dev"
  task = "task2"
    # https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html?icmpid=docs_console_unmapped#rate-based
  schedule = "rate(1 hours)"
  subnet_ids = data.aws_subnets.all.ids
  vpc_id     = data.aws_vpc.default.id
  cluster = module.workloads.ecr_cluster.arn
  allow_public_access = false
  
}
module "event_bus_task_event_task1" {
  source = "../../modules/event_bridge_task"
  project = "instagram"
  env = "dev"
  # https://docs.aws.amazon.com/eventbridge/latest/userguide/eb-events.html
  detail_types = ["SERVICE_DEPLOYMENT_COMPLETED","SERVICE_DEPLOYMENT_FAILED","SERVICE_DEPLOYMENT_IN_PROGRESS"]
  sources = ["service1","service2"]
  rule_name = "hug_all"
  task = "event_task1"
    subnet_ids = data.aws_subnets.all.ids
  vpc_id     = data.aws_vpc.default.id
  cluster = module.workloads.ecr_cluster.arn
  allow_public_access = false
  
}


# Email domain resolved directly in SES module
module "ses" {
  source = "../../modules/ses"
  project = "instagram"
  env     = "dev"
  domain = "mail.dev.instagram.madappgang.com.au"
  
  test_emails = ["i@madappgang.com","ivan.holiak@madappgang.com"]
  zone_id = module.domain.zone_id
}




module "amplify" {
  source = "../../modules/amplify"
  project = "instagram"
  env = "dev"
  
  amplify_apps = [
    {
      name = "main-web"
      github_repository = "https://github.com/username/repo"
      branches = [
        {
          name = "main"
          stage = "PRODUCTION"
          
          enable_auto_build = true
          
          enable_pull_request_preview = true
          
          environment_variables = {
            REACT_APP_COGNITO_REGION = "ap-southeast-2",
            REACT_APP_COGNITO_USER_POOL_ID = "${cognito_user_pool_id}",
            REACT_APP_COGNITO_CLIENT_ID = "${cognito_web_client_id}",
            REACT_APP_ENV = "production",
            REACT_APP_API_URL = "https://api.instagram.madappgang.com.au"
          }
                  },
        {
          name = "develop"
          stage = "DEVELOPMENT"
          
          enable_auto_build = true
          
          enable_pull_request_preview = true
          
          environment_variables = {
            REACT_APP_API_URL = "https://api-dev.instagram.madappgang.com.au",
            REACT_APP_COGNITO_REGION = "ap-southeast-2",
            REACT_APP_COGNITO_USER_POOL_ID = "${cognito_user_pool_id}",
            REACT_APP_COGNITO_CLIENT_ID = "${cognito_web_client_id}",
            REACT_APP_ENV = "development"
          }
                  },
        {
          name = "staging"
          stage = "BETA"
          
          enable_auto_build = true
                    
          environment_variables = {
            REACT_APP_API_URL = "https://api-staging.instagram.madappgang.com.au",
            REACT_APP_COGNITO_REGION = "ap-southeast-2",
            REACT_APP_COGNITO_USER_POOL_ID = "${cognito_user_pool_id}",
            REACT_APP_COGNITO_CLIENT_ID = "${cognito_web_client_id}",
            REACT_APP_ENV = "staging"
          }
                  }
      ]
      custom_domain = "instagram.madappgang.com.au"
      
      enable_root_domain = true
    }
  ]
}


output "backend_ecr_repo_url" {
  value = module.workloads.backend_ecr_repo_url
}

output "account_id" {
  value = module.workloads.account_id
}

output "region" {
  value = data.aws_region.current.name
}

output "backend_task_role_name" {
  value = module.workloads.backend_task_role_name
}

output "backend_cloud_map_arn" {
  description = "value of the backend service discovery ARN"
  value       = module.workloads.backend_cloud_map_arn
}

output "amplify_apps" {
  description = "Map of all Amplify app details including branches"
  value = module.amplify.amplify_apps
}

output "amplify_app_urls" {
  description = "Map of primary URLs to access the Amplify applications"
  value = module.amplify.app_urls
}

output "amplify_branch_urls" {
  description = "Map of all branch URLs for each Amplify app"
  value = module.amplify.branch_urls
}

output "amplify_main-web_app_id" {
  description = "The unique ID of the main-web Amplify app"
  value       = module.amplify.app_ids["main-web"]
}

output "amplify_main-web_app_url" {
  description = "The URL to access the main-web Amplify application"
  value       = module.amplify.app_urls["main-web"]
}

output "amplify_main-web_default_domain" {
  description = "The default domain for the main-web Amplify app"
  value       = module.amplify.default_domains["main-web"]
}

output "amplify_main-web_branches" {
  description = "All branches for the main-web Amplify app"
  value       = module.amplify.amplify_apps["main-web"].branches
}
