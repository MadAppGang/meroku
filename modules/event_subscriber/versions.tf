# Subscribers of the enhanced bus (EventsV2) exist only in the AWS Cloud Control
# provider. Keep the awscc constraint equal to modules/event_bus_enhanced and
# env/main.hbs.
terraform {
  required_version = ">= 1.10.0"

  required_providers {
    awscc = {
      source  = "hashicorp/awscc"
      version = "~> 1.110"
    }
  }
}
