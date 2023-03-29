locals {
  project = "instagram"
  region  = "ap-southeast-2"
  env     = "dev"
  domain  = "instagram.madappgang.com.au"
}

terraform {
  backend "s3" {
    bucket = "${local.project}-terraform-state-dev"
    key    = "${local.env}.tfstate"
    region = local.region 
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
