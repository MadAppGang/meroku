# Pin the AWS provider to the major every generated environment runs. Keep this
# constraint equal to env/main.hbs and ../versions.tf.
terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}
