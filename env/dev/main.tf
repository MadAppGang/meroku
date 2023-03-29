terraform {
  backend "s3" {
    bucket = "muzos-terraform-state-dev"
    key    = "dev.tfstate"
    region = "eu-central-1"
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

  project    = "muzos"
  env        = "dev"
  domain     = "muzos.madappgang.com"
  vpc_id     = data.aws_vpc.default.id
  subnet_ids = data.aws_subnets.all.ids
}

module "cognito" {
  source            = "./../../modules/cognito"
  project           = "muzos"
  env               = "dev"
  domain            = "muzos.madappgang.com"
  enable_web_client = false
}
