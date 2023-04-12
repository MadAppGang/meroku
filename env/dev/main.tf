locals {
  project = "instagram"
  region  = "ap-southeast-2"
  env     = "dev"
  domain  = "instagram.madappgang.com.au"
}

terraform {
  backend "s3" {
    bucket = "instagram-terraform-state-dev"
    key    = "state.tfstate"
    region = ap-southeast-2 
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.25"
    }
  }

  required_version = ">= 1.2.6"
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "all" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}



module "workloads" {
  source = "./../../modules/workloads"

  project    = local.project
  env        = local.env 
  domain     = local.domain 
  private_dns_name = "${local.project}.private"
  vpc_id     = data.aws_vpc.default.id
  subnet_ids = data.aws_subnets.all.ids
}

module "cognito" {
  source            = "./../../modules/cognito"
  project           = local.env
  env               = local.env
  domain            = local.domain
  enable_web_client = false
}

module "postgres" {
  source = "./../../modules/postgres"
  project = local.project
  env = local.env
}

# scheduled taask
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/setting-up.html#setting-up-execution-role
module "task" {
  source = "../../modules/ecs_task"
  project = local.project
  env = local.env
  task = "notify-something"
  subnet_ids = data.aws_subnets.all.ids
  vpc_id     = data.aws_vpc.default.id
  cluster = module.workloads.ecr_cluster.arn
# https://docs.aws.amazon.com/scheduler/latest/UserGuide/schedule-types.html?icmpid=docs_console_unmapped#rate-based
  schedule = "rate(1 minutes)"
}
