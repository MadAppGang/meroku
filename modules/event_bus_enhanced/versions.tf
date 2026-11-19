# The enhanced bus (EventsV2) exists only in the AWS Cloud Control provider;
# RAM sharing and the account and region lookups use the AWS provider. Keep the
# aws constraint equal to env/main.hbs and the awscc one equal to
# modules/event_subscriber.
terraform {
  required_version = ">= 1.10.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    awscc = {
      source  = "hashicorp/awscc"
      version = "~> 1.110"
    }
  }
}
